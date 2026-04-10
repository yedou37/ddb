package raftnode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"

	"github.com/yedou37/dbd/internal/config"
	"github.com/yedou37/dbd/internal/model"
	"github.com/yedou37/dbd/internal/storage"
)

type Node struct {
	raft           *raft.Raft
	logStore       *raftboltdb.BoltStore
	stableStore    *raftboltdb.BoltStore
	transport      *raft.NetworkTransport
	nodeID         string
	raftAddr       string
	mu             sync.RWMutex
	leaderHTTPHint string
}

// #region debug-point B:raft-new
func reportRaftDebugEvent(hypothesisID, location, msg string, data map[string]any) {
	serverURL := "http://127.0.0.1:7777/event"
	sessionID := "ci-node3-restart"
	if content, err := os.ReadFile(".dbg/ci-node3-restart.env"); err == nil {
		for _, line := range strings.Split(string(content), "\n") {
			if value, ok := strings.CutPrefix(line, "DEBUG_SERVER_URL="); ok {
				serverURL = value
			}
			if value, ok := strings.CutPrefix(line, "DEBUG_SESSION_ID="); ok {
				sessionID = value
			}
		}
	}

	payload, err := json.Marshal(map[string]any{
		"sessionId":    sessionID,
		"runId":        "pre-fix",
		"hypothesisId": hypothesisID,
		"location":     location,
		"msg":          msg,
		"data":         data,
		"ts":           time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}

	go func(url string, body []byte) {
		request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err == nil && response != nil {
			_ = response.Body.Close()
		}
	}(serverURL, payload)
}

// #endregion

func New(cfg config.ServerConfig, store *storage.Store) (*Node, error) {
	stateDir := raftStateDir(cfg.RaftDir, cfg.NodeID)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}

	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(cfg.NodeID)

	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:log-store-begin", "[DEBUG] raft log store open begin", map[string]any{
		"nodeID": cfg.NodeID,
		"path":   filepath.Join(stateDir, "raft-log.db"),
	})
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(stateDir, "raft-log.db"))
	if err != nil {
		reportRaftDebugEvent("B", "internal/raftnode/node.go:New:log-store-error", "[DEBUG] raft log store open failed", map[string]any{
			"nodeID": cfg.NodeID,
			"error":  err.Error(),
		})
		return nil, err
	}
	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:log-store-ok", "[DEBUG] raft log store open succeeded", map[string]any{
		"nodeID": cfg.NodeID,
	})

	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:stable-store-begin", "[DEBUG] raft stable store open begin", map[string]any{
		"nodeID": cfg.NodeID,
		"path":   filepath.Join(stateDir, "raft-stable.db"),
	})
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(stateDir, "raft-stable.db"))
	if err != nil {
		reportRaftDebugEvent("B", "internal/raftnode/node.go:New:stable-store-error", "[DEBUG] raft stable store open failed", map[string]any{
			"nodeID": cfg.NodeID,
			"error":  err.Error(),
		})
		return nil, err
	}
	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:stable-store-ok", "[DEBUG] raft stable store open succeeded", map[string]any{
		"nodeID": cfg.NodeID,
	})

	snapshotDir := filepath.Join(stateDir, "snapshots")
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return nil, err
	}

	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:snapshot-store-begin", "[DEBUG] raft snapshot store open begin", map[string]any{
		"nodeID": cfg.NodeID,
		"path":   snapshotDir,
	})
	snapshotStore, err := raft.NewFileSnapshotStore(snapshotDir, 2, io.Discard)
	if err != nil {
		reportRaftDebugEvent("B", "internal/raftnode/node.go:New:snapshot-store-error", "[DEBUG] raft snapshot store open failed", map[string]any{
			"nodeID": cfg.NodeID,
			"error":  err.Error(),
		})
		return nil, err
	}
	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:snapshot-store-ok", "[DEBUG] raft snapshot store open succeeded", map[string]any{
		"nodeID": cfg.NodeID,
	})

	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:transport-begin", "[DEBUG] raft transport open begin", map[string]any{
		"nodeID":   cfg.NodeID,
		"raftAddr": cfg.RaftAddr,
	})
	transport, err := raft.NewTCPTransport(cfg.RaftAddr, nil, 3, 10*time.Second, io.Discard)
	if err != nil {
		reportRaftDebugEvent("B", "internal/raftnode/node.go:New:transport-error", "[DEBUG] raft transport open failed", map[string]any{
			"nodeID": cfg.NodeID,
			"error":  err.Error(),
		})
		return nil, err
	}
	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:transport-ok", "[DEBUG] raft transport open succeeded", map[string]any{
		"nodeID": cfg.NodeID,
	})

	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:raft-begin", "[DEBUG] raft.NewRaft begin", map[string]any{
		"nodeID": cfg.NodeID,
	})
	instance, err := raft.NewRaft(raftConfig, newFSM(store), logStore, stableStore, snapshotStore, transport)
	if err != nil {
		reportRaftDebugEvent("B", "internal/raftnode/node.go:New:raft-error", "[DEBUG] raft.NewRaft failed", map[string]any{
			"nodeID": cfg.NodeID,
			"error":  err.Error(),
		})
		return nil, err
	}
	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:raft-ok", "[DEBUG] raft.NewRaft succeeded", map[string]any{
		"nodeID": cfg.NodeID,
	})

	node := &Node{
		raft:           instance,
		logStore:       logStore,
		stableStore:    stableStore,
		transport:      transport,
		nodeID:         cfg.NodeID,
		raftAddr:       cfg.RaftAddr,
		leaderHTTPHint: normalizeHTTPAddr(cfg.HTTPAddr),
	}

	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:existing-state-begin", "[DEBUG] raft.HasExistingState begin", map[string]any{
		"nodeID": cfg.NodeID,
	})
	hasExistingState, existingStateErr := raft.HasExistingState(logStore, stableStore, snapshotStore)
	if existingStateErr != nil {
		reportRaftDebugEvent("B", "internal/raftnode/node.go:New:existing-state-error", "[DEBUG] raft.HasExistingState failed", map[string]any{
			"nodeID": cfg.NodeID,
			"error":  existingStateErr.Error(),
		})
		return nil, existingStateErr
	}
	reportRaftDebugEvent("B", "internal/raftnode/node.go:New:existing-state-ok", "[DEBUG] raft.HasExistingState succeeded", map[string]any{
		"nodeID":           cfg.NodeID,
		"hasExistingState": hasExistingState,
	})

	if cfg.Bootstrap && !hasExistingState {
		future := instance.BootstrapCluster(raft.Configuration{
			Servers: []raft.Server{{
				ID:      raft.ServerID(cfg.NodeID),
				Address: raft.ServerAddress(cfg.RaftAddr),
			}},
		})
		if err := future.Error(); err != nil && err != raft.ErrCantBootstrap {
			return nil, err
		}
	}

	return node, nil
}

func (n *Node) Apply(sql string, timeout time.Duration) (model.QueryResult, error) {
	future := n.raft.Apply([]byte(sql), timeout)
	if err := future.Error(); err != nil {
		return model.QueryResult{}, err
	}
	return decodeApplyResponse(future.Response())
}

func (n *Node) IsLeader() bool {
	return n.raft.State() == raft.Leader
}

func (n *Node) State() string {
	return n.raft.State().String()
}

func (n *Node) LeaderID() string {
	address := string(n.raft.Leader())
	if address == "" {
		return ""
	}

	future := n.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return ""
	}

	for _, server := range future.Configuration().Servers {
		if string(server.Address) == address {
			return string(server.ID)
		}
	}
	return ""
}

func (n *Node) LeaderRaftAddr() string {
	return string(n.raft.Leader())
}

func (n *Node) Join(nodeID, raftAddr string) error {
	future := n.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return err
	}

	for _, server := range future.Configuration().Servers {
		if server.ID == raft.ServerID(nodeID) || server.Address == raft.ServerAddress(raftAddr) {
			if server.ID == raft.ServerID(nodeID) && server.Address == raft.ServerAddress(raftAddr) {
				return nil
			}
			removeFuture := n.raft.RemoveServer(server.ID, 0, 0)
			if err := removeFuture.Error(); err != nil {
				return err
			}
		}
	}

	addFuture := n.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(raftAddr), 0, 0)
	return addFuture.Error()
}

func (n *Node) Remove(nodeID string) error {
	future := n.raft.RemoveServer(raft.ServerID(nodeID), 0, 0)
	return future.Error()
}

func (n *Node) Members() ([]model.NodeInfo, error) {
	future := n.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return nil, err
	}

	members := make([]model.NodeInfo, 0, len(future.Configuration().Servers))
	for _, server := range future.Configuration().Servers {
		members = append(members, model.NodeInfo{
			ID:       string(server.ID),
			RaftAddr: string(server.Address),
		})
	}
	return members, nil
}

func (n *Node) JoinCluster(ctx context.Context, leaderHTTPAddr, nodeID, raftAddr, httpAddr string) error {
	n.setLeaderHTTPHint(leaderHTTPAddr)

	body, err := json.Marshal(model.JoinRequest{
		NodeID:   nodeID,
		RaftAddr: raftAddr,
		HTTPAddr: httpAddr,
	})
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeHTTPAddr(leaderHTTPAddr)+"/join", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(response.Body)
		return fmt.Errorf("%s", string(payload))
	}

	return nil
}

func (n *Node) RejoinCluster(ctx context.Context, leaderHTTPAddr, nodeID, raftAddr, httpAddr string) error {
	n.setLeaderHTTPHint(leaderHTTPAddr)

	body, err := json.Marshal(model.JoinRequest{
		NodeID:   nodeID,
		RaftAddr: raftAddr,
		HTTPAddr: httpAddr,
	})
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeHTTPAddr(leaderHTTPAddr)+"/rejoin", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(response.Body)
		return fmt.Errorf("%s", string(payload))
	}

	return nil
}

func (n *Node) Close() error {
	var closeErr error
	if n.raft != nil {
		if err := n.raft.Shutdown().Error(); err != nil {
			closeErr = err
		}
	}
	if n.transport != nil {
		if err := n.transport.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if n.logStore != nil {
		if err := n.logStore.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if n.stableStore != nil {
		if err := n.stableStore.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (n *Node) LeaderHTTPHint() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.leaderHTTPHint
}

func (n *Node) setLeaderHTTPHint(value string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.leaderHTTPHint = normalizeHTTPAddr(value)
}

func normalizeHTTPAddr(addr string) string {
	if len(addr) >= 7 && addr[:7] == "http://" {
		return addr
	}
	return "http://" + addr
}

func raftStateDir(base, nodeID string) string {
	return filepath.Join(base, nodeID)
}
