package apiserver

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yedou37/ddb/internal/controller"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/shardmeta"
)

//go:embed dashboard
var dashboardAssets embed.FS

type dashboardOverview struct {
	GeneratedAt  time.Time               `json:"generated_at"`
	Summary      dashboardSummary        `json:"summary"`
	Config       shardmeta.ClusterConfig `json:"config"`
	Shards       model.ShardsResponse    `json:"shards"`
	LockedShards []uint32                `json:"locked_shards"`
	Nodes        []dashboardNode         `json:"nodes"`
	Groups       []dashboardGroup        `json:"groups"`
	Errors       []string                `json:"errors,omitempty"`
}

type dashboardSummary struct {
	TotalNodes      int    `json:"total_nodes"`
	ReachableNodes  int    `json:"reachable_nodes"`
	ShardNodes      int    `json:"shard_nodes"`
	ControllerNodes int    `json:"controller_nodes"`
	APIServerNodes  int    `json:"apiserver_nodes"`
	GroupCount      int    `json:"group_count"`
	HealthyGroups   int    `json:"healthy_groups"`
	MigratingGroups int    `json:"migrating_groups"`
	ConfigVersion   uint64 `json:"config_version"`
	TotalShards     int    `json:"total_shards"`
}

type dashboardNode struct {
	ID            string   `json:"id"`
	Role          string   `json:"role"`
	GroupID       string   `json:"group_id,omitempty"`
	HTTPAddr      string   `json:"http_addr,omitempty"`
	RaftAddr      string   `json:"raft_addr,omitempty"`
	IsLeader      bool     `json:"is_leader"`
	Reachable     bool     `json:"reachable"`
	Status        string   `json:"status"`
	ClusterLeader string   `json:"cluster_leader,omitempty"`
	Tables        []string `json:"tables,omitempty"`
	TableCount    int      `json:"table_count"`
	LastError     string   `json:"last_error,omitempty"`
}

type dashboardGroup struct {
	GroupID        string               `json:"group_id"`
	Status         string               `json:"status"`
	ShardCount     int                  `json:"shard_count"`
	Shards         []uint32             `json:"shards"`
	NodeCount      int                  `json:"node_count"`
	ReachableNodes int                  `json:"reachable_nodes"`
	LeaderNodeID   string               `json:"leader_node_id,omitempty"`
	Nodes          []dashboardNodeBrief `json:"nodes"`
}

type dashboardNodeBrief struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Reachable bool   `json:"reachable"`
	IsLeader  bool   `json:"is_leader"`
	Status    string `json:"status"`
}

type RemovedIDLister interface {
	ListRemovedIDs(ctx context.Context) ([]string, error)
}

type dashboardState struct {
	mu       sync.Mutex
	lastSeen map[string]dashboardNode
}

func registerDashboardRoutes(mux *http.ServeMux, service *controller.Service, nodeLister NodeLister) {
	state := &dashboardState{lastSeen: make(map[string]dashboardNode)}
	mux.HandleFunc("/dashboard/api/overview", func(w http.ResponseWriter, r *http.Request) {
		overview := buildDashboardOverview(r.Context(), service, nodeLister, state)
		writeJSON(w, http.StatusOK, overview)
	})
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusTemporaryRedirect)
	})

	staticFS, err := fs.Sub(dashboardAssets, "dashboard")
	if err != nil {
		mux.HandleFunc("/dashboard/", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		})
		return
	}
	mux.Handle("/dashboard/", http.StripPrefix("/dashboard/", http.FileServer(http.FS(staticFS))))
}

func buildDashboardOverview(ctx context.Context, service *controller.Service, nodeLister NodeLister, state *dashboardState) dashboardOverview {
	overview := dashboardOverview{
		GeneratedAt:  time.Now(),
		LockedShards: []uint32{},
		Nodes:        []dashboardNode{},
		Groups:       []dashboardGroup{},
		Errors:       []string{},
	}

	if service != nil {
		config := service.CurrentConfig()
		overview.Config = config
		overview.Shards = buildShardsResponse(config)
		for _, shardID := range service.LockedShardIDs() {
			overview.LockedShards = append(overview.LockedShards, uint32(shardID))
		}
	}

	var rawNodes []model.NodeInfo
	if nodeLister != nil {
		nodes, err := nodeLister.ListNodes(ctx)
		if err != nil {
			overview.Errors = append(overview.Errors, err.Error())
		} else {
			rawNodes = nodes
		}
	}

	removedIDs := []string{}
	if removedLister, ok := nodeLister.(RemovedIDLister); ok {
		ids, err := removedLister.ListRemovedIDs(ctx)
		if err != nil {
			overview.Errors = append(overview.Errors, err.Error())
		} else {
			removedIDs = ids
		}
	}

	overview.Nodes = buildDashboardNodes(rawNodes, removedIDs, state)
	overview.Groups = buildDashboardGroups(overview.Config, overview.Nodes, overview.LockedShards)
	overview.Summary = buildDashboardSummary(overview)
	return overview
}

func buildDashboardNodes(rawNodes []model.NodeInfo, removedIDs []string, state *dashboardState) []dashboardNode {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	nodes := make([]dashboardNode, 0, len(rawNodes))
	currentByID := make(map[string]dashboardNode, len(rawNodes))
	for _, node := range rawNodes {
		item := dashboardNode{
			ID:       node.ID,
			Role:     strings.TrimSpace(node.Role),
			GroupID:  node.GroupID,
			HTTPAddr: node.HTTPAddr,
			RaftAddr: node.RaftAddr,
			IsLeader: node.IsLeader,
			Status:   "unknown",
		}

		baseURL := ensureHTTPURL(node.HTTPAddr)
		if baseURL != "" {
			reachable, errText := probeHealth(client, baseURL+"/health")
			item.Reachable = reachable
			if errText != "" {
				item.LastError = errText
			}
			if reachable {
				item.Status = "online"
			} else {
				item.Status = "offline"
			}

			if item.Role == string(shardmeta.RoleShardNode) && reachable {
				status, errText := fetchNodeStatus(client, baseURL+"/status")
				if errText != "" {
					item.LastError = errText
				} else {
					item.ClusterLeader = status.Leader
					item.Tables = status.Tables
					if item.Tables == nil {
						item.Tables = []string{}
					}
					item.TableCount = len(status.Tables)
				}
			}
		}

		nodes = append(nodes, item)
		currentByID[item.ID] = item
	}

	removedSet := make(map[string]struct{}, len(removedIDs))
	for _, id := range removedIDs {
		removedSet[id] = struct{}{}
	}

	if state != nil {
		state.mu.Lock()
		for id := range removedSet {
			delete(state.lastSeen, id)
		}
		maps.Copy(state.lastSeen, currentByID)
		for id, cached := range state.lastSeen {
			if _, ok := removedSet[id]; ok {
				continue
			}
			if _, ok := currentByID[id]; ok {
				continue
			}
			cached.Reachable = false
			cached.IsLeader = false
			cached.Status = "offline"
			if cached.LastError == "" {
				cached.LastError = "node missing from discovery"
			}
			nodes = append(nodes, cached)
		}
		state.mu.Unlock()
	}

	slices.SortFunc(nodes, func(a, b dashboardNode) int {
		if a.Role != b.Role {
			return strings.Compare(a.Role, b.Role)
		}
		if a.GroupID != b.GroupID {
			return strings.Compare(a.GroupID, b.GroupID)
		}
		return strings.Compare(a.ID, b.ID)
	})
	return nodes
}

func buildDashboardGroups(config shardmeta.ClusterConfig, nodes []dashboardNode, lockedShards []uint32) []dashboardGroup {
	shardsByGroup := make(map[string][]uint32)
	for _, assignment := range config.Assignments {
		groupID := string(assignment.GroupID)
		shardsByGroup[groupID] = append(shardsByGroup[groupID], uint32(assignment.ShardID))
	}

	nodesByGroup := make(map[string][]dashboardNode)
	for _, node := range nodes {
		if node.Role != string(shardmeta.RoleShardNode) || node.GroupID == "" {
			continue
		}
		nodesByGroup[node.GroupID] = append(nodesByGroup[node.GroupID], node)
	}

	groupIDSet := make(map[string]struct{})
	for groupID := range shardsByGroup {
		groupIDSet[groupID] = struct{}{}
	}
	for groupID := range nodesByGroup {
		groupIDSet[groupID] = struct{}{}
	}

	groupIDs := make([]string, 0, len(groupIDSet))
	for groupID := range groupIDSet {
		groupIDs = append(groupIDs, groupID)
	}
	slices.Sort(groupIDs)

	lockedSet := make(map[uint32]struct{}, len(lockedShards))
	for _, shardID := range lockedShards {
		lockedSet[shardID] = struct{}{}
	}

	groups := make([]dashboardGroup, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		groupNodes := nodesByGroup[groupID]
		groupShards := shardsByGroup[groupID]
		if groupShards == nil {
			groupShards = []uint32{}
		}
		reachable := 0
		leaderNodeID := ""
		briefs := make([]dashboardNodeBrief, 0, len(groupNodes))
		for _, node := range groupNodes {
			if node.Reachable {
				reachable++
			}
			if node.IsLeader {
				leaderNodeID = node.ID
			}
			briefs = append(briefs, dashboardNodeBrief{
				ID:        node.ID,
				Role:      node.Role,
				Reachable: node.Reachable,
				IsLeader:  node.IsLeader,
				Status:    node.Status,
			})
		}

		status := "healthy"
		if reachable == 0 {
			status = "offline"
		} else if leaderNodeID == "" {
			status = "degraded"
		}
		for _, shardID := range groupShards {
			if _, ok := lockedSet[shardID]; ok {
				status = "migrating"
				break
			}
		}

		slices.Sort(groupShards)
		groups = append(groups, dashboardGroup{
			GroupID:        groupID,
			Status:         status,
			ShardCount:     len(groupShards),
			Shards:         groupShards,
			NodeCount:      len(groupNodes),
			ReachableNodes: reachable,
			LeaderNodeID:   leaderNodeID,
			Nodes:          briefs,
		})
	}
	return groups
}

func buildDashboardSummary(overview dashboardOverview) dashboardSummary {
	summary := dashboardSummary{
		ConfigVersion: overview.Config.Version,
		TotalShards:   overview.Config.TotalShards,
		GroupCount:    len(overview.Groups),
		TotalNodes:    len(overview.Nodes),
	}
	for _, node := range overview.Nodes {
		if node.Reachable {
			summary.ReachableNodes++
		}
		switch node.Role {
		case string(shardmeta.RoleShardNode):
			summary.ShardNodes++
		case string(shardmeta.RoleController):
			summary.ControllerNodes++
		case string(shardmeta.RoleAPIServer):
			summary.APIServerNodes++
		}
	}
	for _, group := range overview.Groups {
		switch group.Status {
		case "healthy":
			summary.HealthyGroups++
		case "migrating":
			summary.MigratingGroups++
		}
	}
	return summary
}

func ensureHTTPURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return "http://" + value
}

func probeHealth(client *http.Client, url string) (bool, string) {
	resp, err := client.Get(url)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, resp.Status
	}
	return true, ""
}

func fetchNodeStatus(client *http.Client, url string) (model.StatusResponse, string) {
	resp, err := client.Get(url)
	if err != nil {
		return model.StatusResponse{}, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return model.StatusResponse{}, resp.Status
	}
	var status model.StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return model.StatusResponse{}, err.Error()
	}
	return status, ""
}
