package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/yedou37/dbd/internal/discovery"
	"github.com/yedou37/dbd/internal/model"
	"github.com/yedou37/dbd/internal/raftnode"
	sqlparser "github.com/yedou37/dbd/internal/sql"
	"github.com/yedou37/dbd/internal/storage"
)

type QueryService struct {
	nodeID    string
	httpAddr  string
	raftAddr  string
	store     *storage.Store
	raftNode  *raftnode.Node
	discovery *discovery.Client
}

type LeaderRedirectError struct {
	Leader string
}

func (e *LeaderRedirectError) Error() string {
	if e.Leader == "" {
		return "write request must be sent to leader"
	}
	return fmt.Sprintf("write request must be sent to leader %s", e.Leader)
}

func NewQueryService(nodeID, httpAddr, raftAddr string, store *storage.Store, raftNode *raftnode.Node, discoveryClient *discovery.Client) *QueryService {
	return &QueryService{
		nodeID:    nodeID,
		httpAddr:  httpAddr,
		raftAddr:  raftAddr,
		store:     store,
		raftNode:  raftNode,
		discovery: discoveryClient,
	}
}

func (s *QueryService) Execute(ctx context.Context, input string) (model.QueryResult, error) {
	statement, err := sqlparser.Parse(input)
	if err != nil {
		return model.QueryResult{}, err
	}

	if isWrite(statement.Type) {
		if s.raftNode == nil {
			return s.store.ExecuteStatement(statement)
		}
		if !s.raftNode.IsLeader() {
			leader, _ := s.leaderAddr(ctx)
			return model.QueryResult{}, &LeaderRedirectError{Leader: leader}
		}
		return s.raftNode.Apply(input, 10*time.Second)
	}

	if statement.Type == model.StatementSelect || statement.Type == model.StatementShowTables {
		return s.store.ExecuteStatement(statement)
	}

	return model.QueryResult{}, errors.New("unsupported statement")
}

func (s *QueryService) Status(ctx context.Context) (model.StatusResponse, error) {
	tables, err := s.store.ListTables()
	if err != nil {
		return model.StatusResponse{}, err
	}

	role := "standalone"
	if s.raftNode != nil {
		role = s.raftNode.State()
	}

	leader, _ := s.leaderAddr(ctx)
	if leader == "" && (s.raftNode == nil || s.raftNode.IsLeader()) {
		leader = s.httpAddr
	}

	return model.StatusResponse{
		NodeID:   s.nodeID,
		HTTPAddr: s.httpAddr,
		Role:     role,
		Leader:   leader,
		Tables:   tables,
	}, nil
}

func (s *QueryService) Members(ctx context.Context) ([]model.NodeInfo, error) {
	if s.discovery == nil {
		return []model.NodeInfo{{
			ID:       s.nodeID,
			RaftAddr: s.raftAddr,
			HTTPAddr: s.httpAddr,
			IsLeader: s.raftNode == nil || s.raftNode.IsLeader(),
		}}, nil
	}
	return s.discovery.ListNodes(ctx)
}

func (s *QueryService) Leader(ctx context.Context) (model.NodeInfo, error) {
	if s.raftNode == nil {
		return model.NodeInfo{ID: s.nodeID, RaftAddr: s.raftAddr, HTTPAddr: s.httpAddr, IsLeader: true}, nil
	}

	if s.raftNode.IsLeader() {
		return model.NodeInfo{ID: s.nodeID, RaftAddr: s.raftAddr, HTTPAddr: s.httpAddr, IsLeader: true}, nil
	}

	leaderID := s.raftNode.LeaderID()
	if leaderID == "" {
		return model.NodeInfo{}, errors.New("leader not elected")
	}

	if s.discovery != nil {
		nodes, err := s.discovery.ListNodes(ctx)
		if err == nil {
			for _, node := range nodes {
				if node.ID == leaderID {
					node.IsLeader = true
					return node, nil
				}
			}
		}
	}

	if hint := s.raftNode.LeaderHTTPHint(); hint != "" {
		return model.NodeInfo{
			ID:       leaderID,
			HTTPAddr: hint,
			IsLeader: leaderID == s.nodeID,
		}, nil
	}

	return model.NodeInfo{}, errors.New("leader http address not found")
}

func (s *QueryService) leaderAddr(ctx context.Context) (string, error) {
	leader, err := s.Leader(ctx)
	if err != nil {
		return "", err
	}
	return leader.HTTPAddr, nil
}

func isWrite(statementType model.StatementType) bool {
	return statementType == model.StatementCreateTable || statementType == model.StatementInsert || statementType == model.StatementDelete
}

func (s *QueryService) Join(_ context.Context, request model.JoinRequest) error {
	if s.raftNode == nil {
		return errors.New("raft is not enabled")
	}
	if !s.raftNode.IsLeader() {
		leader, _ := s.leaderAddr(context.Background())
		return &LeaderRedirectError{Leader: leader}
	}
	return s.raftNode.Join(request.NodeID, request.RaftAddr)
}
