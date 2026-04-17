package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/yedou37/ddb/internal/controller"
	"github.com/yedou37/ddb/internal/coordinator"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/shardmeta"
)

type MoveShardRequest struct {
	ShardID shardmeta.ShardID `json:"shard_id"`
	GroupID shardmeta.GroupID `json:"group_id"`
}

type RebalanceRequest struct {
	GroupIDs []shardmeta.GroupID `json:"group_ids"`
}

type SQLExecutor interface {
	ExecuteSQL(ctx context.Context, input string) (model.SQLResponse, error)
}

type ShardMigrator interface {
	MigrateShard(ctx context.Context, shardID shardmeta.ShardID, sourceGroup, targetGroup shardmeta.GroupID) error
}

type NodeLister interface {
	ListNodes(ctx context.Context) ([]model.NodeInfo, error)
}

func NewHandler(service *controller.Service, nodeLister NodeLister, executor SQLExecutor, migrator ShardMigrator) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/sql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, model.SQLResponse{Success: false, Error: "method not allowed"})
			return
		}
		if executor == nil {
			writeJSON(w, http.StatusServiceUnavailable, model.SQLResponse{Success: false, Error: coordinator.ErrCoordinatorUnavailable.Error()})
			return
		}

		var request model.SQLRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, model.SQLResponse{Success: false, Error: err.Error()})
			return
		}

		response, err := executor.ExecuteSQL(r.Context(), request.SQL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, model.SQLResponse{Success: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("/config", func(w http.ResponseWriter, _ *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "controller service is not configured"})
			return
		}
		writeJSON(w, http.StatusOK, service.CurrentConfig())
	})
	mux.HandleFunc("/shards", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "controller service is not configured"})
			return
		}
		writeJSON(w, http.StatusOK, buildShardsResponse(service.CurrentConfig()))
	})
	mux.HandleFunc("/groups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "controller service is not configured"})
			return
		}

		var nodes []model.NodeInfo
		if nodeLister != nil {
			listedNodes, err := nodeLister.ListNodes(r.Context())
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
			nodes = listedNodes
		}
		writeJSON(w, http.StatusOK, buildGroupStatuses(service.CurrentConfig(), nodes))
	})
	mux.HandleFunc("/move-shard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "controller service is not configured"})
			return
		}

		var request MoveShardRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		current := service.CurrentConfig()
		sourceGroup, ok := current.GroupForShard(request.ShardID)
		if !ok {
			writeControllerError(w, controller.ErrShardNotFound)
			return
		}
		if migrator != nil && sourceGroup != request.GroupID {
			if err := migrator.MigrateShard(r.Context(), request.ShardID, sourceGroup, request.GroupID); err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
		}

		nextConfig, err := service.PreviewMoveShard(request.ShardID, request.GroupID)
		if err != nil {
			writeControllerError(w, err)
			return
		}
		config, err := service.UpdateConfig(nextConfig)
		if err != nil {
			writeControllerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, config)
	})
	mux.HandleFunc("/rebalance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "controller service is not configured"})
			return
		}

		var request RebalanceRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		current := service.CurrentConfig()
		nextConfig, err := service.PreviewRebalance(request.GroupIDs)
		if err != nil {
			writeControllerError(w, err)
			return
		}
		if migrator != nil {
			for _, movement := range diffAssignments(current, nextConfig) {
				if err := migrator.MigrateShard(r.Context(), movement.ShardID, movement.FromGroup, movement.ToGroup); err != nil {
					writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
					return
				}
			}
		}
		config, err := service.UpdateConfig(nextConfig)
		if err != nil {
			writeControllerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, config)
	})
	return mux
}

func writeControllerError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, controller.ErrShardNotFound) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func buildShardsResponse(config shardmeta.ClusterConfig) model.ShardsResponse {
	assignments := make([]model.ShardStatus, 0, len(config.Assignments))
	for _, assignment := range config.Assignments {
		assignments = append(assignments, model.ShardStatus{
			ShardID: uint32(assignment.ShardID),
			GroupID: string(assignment.GroupID),
		})
	}
	slices.SortFunc(assignments, func(a, b model.ShardStatus) int {
		switch {
		case a.ShardID < b.ShardID:
			return -1
		case a.ShardID > b.ShardID:
			return 1
		default:
			return 0
		}
	})
	return model.ShardsResponse{
		Version:     config.Version,
		TotalShards: config.TotalShards,
		Assignments: assignments,
	}
}

func buildGroupStatuses(config shardmeta.ClusterConfig, nodes []model.NodeInfo) []model.GroupStatus {
	shardsByGroup := make(map[string][]uint32)
	for _, assignment := range config.Assignments {
		groupID := string(assignment.GroupID)
		shardsByGroup[groupID] = append(shardsByGroup[groupID], uint32(assignment.ShardID))
	}

	nodesByGroup := make(map[string][]model.NodeInfo)
	for _, node := range nodes {
		if node.Role != "" && node.Role != string(shardmeta.RoleShardNode) {
			continue
		}
		if node.GroupID == "" {
			continue
		}
		nodesByGroup[node.GroupID] = append(nodesByGroup[node.GroupID], node)
	}

	groupIDSet := make(map[string]bool)
	for groupID := range shardsByGroup {
		groupIDSet[groupID] = true
	}
	for groupID := range nodesByGroup {
		groupIDSet[groupID] = true
	}

	groupIDs := make([]string, 0, len(groupIDSet))
	for groupID := range groupIDSet {
		groupIDs = append(groupIDs, groupID)
	}
	slices.Sort(groupIDs)

	statuses := make([]model.GroupStatus, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		shards := shardsByGroup[groupID]
		slices.Sort(shards)
		groupNodes := nodesByGroup[groupID]
		slices.SortFunc(groupNodes, func(a, b model.NodeInfo) int {
			switch {
			case a.ID < b.ID:
				return -1
			case a.ID > b.ID:
				return 1
			default:
				return 0
			}
		})
		statuses = append(statuses, model.GroupStatus{
			GroupID:    groupID,
			ShardCount: len(shards),
			Shards:     shards,
			Nodes:      groupNodes,
		})
	}
	return statuses
}

type shardMovement struct {
	ShardID   shardmeta.ShardID
	FromGroup shardmeta.GroupID
	ToGroup   shardmeta.GroupID
}

func diffAssignments(current, next shardmeta.ClusterConfig) []shardMovement {
	movements := make([]shardMovement, 0)
	for _, assignment := range current.Assignments {
		targetGroup, ok := next.GroupForShard(assignment.ShardID)
		if !ok || targetGroup == assignment.GroupID {
			continue
		}
		movements = append(movements, shardMovement{
			ShardID:   assignment.ShardID,
			FromGroup: assignment.GroupID,
			ToGroup:   targetGroup,
		})
	}
	return movements
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
