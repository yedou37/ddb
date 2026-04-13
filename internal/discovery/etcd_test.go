package discovery

import (
	"context"
	"testing"

	"github.com/yedou37/ddb/internal/model"
)

func TestNilClientBehaviors(t *testing.T) {
	var client *Client

	if err := client.Register(context.Background(), model.NodeInfo{ID: "node1"}); err != nil {
		t.Fatalf("Register(nil) error = %v", err)
	}
	if err := client.Update(context.Background(), model.NodeInfo{ID: "node1"}); err != nil {
		t.Fatalf("Update(nil) error = %v", err)
	}
	if err := client.MarkRemoved(context.Background(), "node1"); err != nil {
		t.Fatalf("MarkRemoved(nil) error = %v", err)
	}
	if err := client.UnmarkRemoved(context.Background(), "node1"); err != nil {
		t.Fatalf("UnmarkRemoved(nil) error = %v", err)
	}

	nodes, err := client.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes(nil) error = %v", err)
	}
	if nodes != nil {
		t.Fatalf("ListNodes(nil) = %#v, want nil", nodes)
	}

	removed, err := client.IsRemoved(context.Background(), "node1")
	if err != nil {
		t.Fatalf("IsRemoved(nil) error = %v", err)
	}
	if removed {
		t.Fatalf("IsRemoved(nil) = true, want false")
	}

	removedIDs, err := client.ListRemovedIDs(context.Background())
	if err != nil {
		t.Fatalf("ListRemovedIDs(nil) error = %v", err)
	}
	if removedIDs != nil {
		t.Fatalf("ListRemovedIDs(nil) = %#v, want nil", removedIDs)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close(nil) error = %v", err)
	}
}

func TestNewWithoutEndpoints(t *testing.T) {
	client, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) error = %v", err)
	}
	if client != nil {
		t.Fatalf("New(nil) = %#v, want nil", client)
	}
}

func TestFindLeaderWithoutNodes(t *testing.T) {
	client := &Client{}

	leader, err := client.FindLeader(context.Background())
	if err == nil {
		t.Fatalf("FindLeader() error = nil, want error")
	}
	if leader != nil {
		t.Fatalf("FindLeader() leader = %#v, want nil", leader)
	}
}

func TestSetLastNode(t *testing.T) {
	client := &Client{}
	client.setLastNode(model.NodeInfo{ID: "node1", HTTPAddr: "127.0.0.1:8080"})

	if client.lastNode == nil {
		t.Fatalf("client.lastNode = nil, want non-nil")
	}
	if got, want := client.lastNode.ID, "node1"; got != want {
		t.Fatalf("client.lastNode.ID = %q, want %q", got, want)
	}
}
