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
	"sync"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"

	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/storage"
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

func New(cfg config.ServerConfig, store *storage.Store) (*Node, error) {
	stateDir := raftStateDir(cfg.RaftDir, cfg.NodeID)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}

	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(cfg.NodeID)

	logStore, err := raftboltdb.NewBoltStore(filepath.Join(stateDir, "raft-log.db"))
	if err != nil {
		return nil, err
	}

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(stateDir, "raft-stable.db"))
	if err != nil {
		return nil, err
	}

	snapshotDir := filepath.Join(stateDir, "snapshots")
	if mkdirErr := os.MkdirAll(snapshotDir, 0o755); mkdirErr != nil {
		return nil, mkdirErr
	}

	snapshotStore, err := raft.NewFileSnapshotStore(snapshotDir, 2, io.Discard)
	if err != nil {
		return nil, err
	}

	transport, err := raft.NewTCPTransport(cfg.RaftAddr, nil, 3, 10*time.Second, io.Discard)
	if err != nil {
		return nil, err
	}

	instance, err := raft.NewRaft(raftConfig, newFSM(store), logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, err
	}

	node := &Node{
		raft:           instance,
		logStore:       logStore,
		stableStore:    stableStore,
		transport:      transport,
		nodeID:         cfg.NodeID,
		raftAddr:       cfg.RaftAddr,
		leaderHTTPHint: normalizeHTTPAddr(cfg.HTTPAddr),
	}

	hasExistingState, existingStateErr := raft.HasExistingState(logStore, stableStore, snapshotStore)
	if existingStateErr != nil {
		return nil, existingStateErr
	}

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
