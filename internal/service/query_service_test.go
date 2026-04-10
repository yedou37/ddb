package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yedou37/ddb/internal/model"
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
}

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()

	store, err := storage.Open(filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	return store
}
