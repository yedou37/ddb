package service

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/raftnode"
	"github.com/yedou37/ddb/internal/storage"
)

func TestQueryServiceStandaloneFlow(t *testing.T) {
	store := openTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	service := NewQueryService("node1", "127.0.0.1:8080", "127.0.0.1:7000", store, nil, nil)

	if _, err := service.Execute(context.Background(), "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("Execute(create) error = %v", err)
	}
	if _, err := service.Execute(context.Background(), "INSERT INTO users VALUES (1, 'alice')"); err != nil {
		t.Fatalf("Execute(insert) error = %v", err)
	}

	result, err := service.Execute(context.Background(), "SELECT * FROM users WHERE id = 1")
	if err != nil {
		t.Fatalf("Execute(select) error = %v", err)
	}
	if got, want := len(result.Rows), 1; got != want {
		t.Fatalf("len(result.Rows) = %d, want %d", got, want)
	}
	if got, want := result.Rows[0][1], "alice"; got != want {
		t.Fatalf("result.Rows[0][1] = %#v, want %#v", got, want)
	}
}

func TestQueryServiceStatusAndMembersStandalone(t *testing.T) {
	store := openTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	service := NewQueryService("node1", "127.0.0.1:8080", "127.0.0.1:7000", store, nil, nil)

	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if got, want := status.Role, "standalone"; got != want {
		t.Fatalf("status.Role = %q, want %q", got, want)
	}
	if got, want := status.Leader, "127.0.0.1:8080"; got != want {
		t.Fatalf("status.Leader = %q, want %q", got, want)
	}

	members, err := service.Members(context.Background())
	if err != nil {
		t.Fatalf("Members() error = %v", err)
	}
	if got, want := len(members), 1; got != want {
		t.Fatalf("len(members) = %d, want %d", got, want)
	}
	if got, want := members[0].Status, "online-voter"; got != want {
		t.Fatalf("members[0].Status = %q, want %q", got, want)
	}

	leader, err := service.Leader(context.Background())
	if err != nil {
		t.Fatalf("Leader() error = %v", err)
	}
	if !leader.IsLeader {
		t.Fatalf("leader.IsLeader = false, want true")
	}
}

func TestQueryServiceRejoinWithoutRaft(t *testing.T) {
	store := openTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	service := NewQueryService("node1", "127.0.0.1:8080", "127.0.0.1:7000", store, nil, nil)

	if err := service.Join(context.Background(), model.JoinRequest{
		NodeID:   "node2",
		RaftAddr: "127.0.0.1:7001",
		HTTPAddr: "127.0.0.1:8081",
	}); err == nil {
		t.Fatalf("Join() error = nil, want error")
	}

	if err := service.Remove(context.Background(), model.RemoveRequest{NodeID: "node2"}); err == nil {
		t.Fatalf("Remove() error = nil, want error")
	}

	if err := service.Rejoin(context.Background(), model.JoinRequest{
		NodeID:   "node2",
		RaftAddr: "127.0.0.1:7001",
	}); err == nil {
		t.Fatalf("Rejoin() error = nil, want error")
	}
}

func TestQueryServiceStandaloneShowTablesAndDelete(t *testing.T) {
	store := openTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	service := NewQueryService("node1", "127.0.0.1:8080", "127.0.0.1:7000", store, nil, nil)

	if _, err := service.Execute(context.Background(), "CREATE TABLE books (id INT PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("Execute(create) error = %v", err)
	}
	if _, err := service.Execute(context.Background(), "INSERT INTO books VALUES (1, 'raft')"); err != nil {
		t.Fatalf("Execute(insert) error = %v", err)
	}

	tables, err := service.Execute(context.Background(), "SHOW TABLES")
	if err != nil {
		t.Fatalf("Execute(show tables) error = %v", err)
	}
	if got, want := len(tables.Tables), 1; got != want {
		t.Fatalf("len(tables.Tables) = %d, want %d", got, want)
	}
	if got, want := tables.Tables[0], "books"; got != want {
		t.Fatalf("tables.Tables[0] = %q, want %q", got, want)
	}

	deleteResult, err := service.Execute(context.Background(), "DELETE FROM books WHERE id = 1")
	if err != nil {
		t.Fatalf("Execute(delete) error = %v", err)
	}
	if got, want := deleteResult.RowsAffected, 1; got != want {
		t.Fatalf("deleteResult.RowsAffected = %d, want %d", got, want)
	}

	selectResult, err := service.Execute(context.Background(), "SELECT * FROM books")
	if err != nil {
		t.Fatalf("Execute(select after delete) error = %v", err)
	}
	if got := len(selectResult.Rows); got != 0 {
		t.Fatalf("len(selectResult.Rows) = %d, want 0", got)
	}
}

func TestLeaderRedirectErrorAndIsWrite(t *testing.T) {
	if got, want := (&LeaderRedirectError{}).Error(), "write request must be sent to leader"; got != want {
		t.Fatalf("LeaderRedirectError().Error() = %q, want %q", got, want)
	}
	if got, want := (&LeaderRedirectError{Leader: "http://leader:8080"}).Error(), "write request must be sent to leader http://leader:8080"; got != want {
		t.Fatalf("LeaderRedirectError(leader).Error() = %q, want %q", got, want)
	}

	if !isWrite(model.StatementCreateTable) || !isWrite(model.StatementInsert) || !isWrite(model.StatementDelete) {
		t.Fatalf("isWrite() returned false for a write statement")
	}
	if isWrite(model.StatementSelect) || isWrite(model.StatementShowTables) {
		t.Fatalf("isWrite() returned true for a read statement")
	}
}

func TestQueryServiceRaftLeaderFlow(t *testing.T) {
	store, node := newTestRaftNode(t)
	defer func() {
		_ = node.Close()
		_ = store.Close()
	}()

	service := NewQueryService("node1", "127.0.0.1:8080", node.LeaderRaftAddr(), store, node, nil)

	if _, err := service.Execute(context.Background(), "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("Execute(create via raft) error = %v", err)
	}
	if _, err := service.Execute(context.Background(), "INSERT INTO users VALUES (1, 'alice')"); err != nil {
		t.Fatalf("Execute(insert via raft) error = %v", err)
	}
	if _, err := service.Execute(context.Background(), "DELETE FROM users WHERE id = 1"); err != nil {
		t.Fatalf("Execute(delete via raft) error = %v", err)
	}

	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Leader != "127.0.0.1:8080" {
		t.Fatalf("status.Leader = %q, want %q", status.Leader, "127.0.0.1:8080")
	}

	leader, err := service.Leader(context.Background())
	if err != nil {
		t.Fatalf("Leader() error = %v", err)
	}
	if !leader.IsLeader || leader.ID != "node1" {
		t.Fatalf("Leader() = %#v, want current leader node", leader)
	}

	members, err := service.Members(context.Background())
	if err != nil {
		t.Fatalf("Members() error = %v", err)
	}
	if got, want := len(members), 1; got != want {
		t.Fatalf("len(members) = %d, want %d", got, want)
	}
	if !members[0].InRaft {
		t.Fatalf("members[0].InRaft = false, want true")
	}

	if err := service.Remove(context.Background(), model.RemoveRequest{NodeID: "node1"}); err == nil {
		t.Fatalf("Remove(current leader) error = nil, want error")
	}
	if err := service.Rejoin(context.Background(), model.JoinRequest{NodeID: "node2"}); err == nil {
		t.Fatalf("Rejoin(missing raft addr) error = nil, want error")
	}
}

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()

	store, err := storage.Open(filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	return store
}

func newTestRaftNode(t *testing.T) (*storage.Store, *raftnode.Node) {
	t.Helper()

	baseDir := t.TempDir()
	store, err := storage.Open(filepath.Join(baseDir, "service.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}

	cfg := config.ServerConfig{
		NodeID:    "node1",
		HTTPAddr:  "127.0.0.1:8080",
		RaftAddr:  reserveAddr(t),
		RaftDir:   filepath.Join(baseDir, "raft"),
		Bootstrap: true,
	}

	node, err := raftnode.New(cfg, store)
	if err != nil {
		_ = store.Close()
		t.Fatalf("raftnode.New() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if node.IsLeader() {
			return store, node
		}
		time.Sleep(50 * time.Millisecond)
	}

	_ = node.Close()
	_ = store.Close()
	t.Fatalf("raft node did not become leader before timeout")
	return nil, nil
}

func reserveAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer listener.Close()
	return listener.Addr().String()
}
