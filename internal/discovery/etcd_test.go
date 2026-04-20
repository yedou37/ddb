package discovery

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/shardmeta"
	embedetcd "go.etcd.io/etcd/server/v3/embed"
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

func TestEmbeddedClientRegisterUpdateAndLeaseLifecycle(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	client1, err := New([]string{etcd.clientEndpoint})
	if err != nil {
		t.Fatalf("New(client1) error = %v", err)
	}
	defer func() {
		_ = client1.Close()
	}()
	client2, err := New([]string{etcd.clientEndpoint})
	if err != nil {
		t.Fatalf("New(client2) error = %v", err)
	}
	defer func() {
		_ = client2.Close()
	}()

	node1 := model.NodeInfo{ID: "node1", HTTPAddr: "127.0.0.1:18080", RaftAddr: "127.0.0.1:28080", IsLeader: true}
	node2 := model.NodeInfo{ID: "node2", HTTPAddr: "127.0.0.1:18081", RaftAddr: "127.0.0.1:28081"}
	if err := client1.Register(context.Background(), node1); err != nil {
		t.Fatalf("Register(node1) error = %v", err)
	}
	if err := client2.Register(context.Background(), node2); err != nil {
		t.Fatalf("Register(node2) error = %v", err)
	}

	waitForNodeCount(t, client1, 2, 10*time.Second)

	nodes, err := client1.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if got, want := nodes[0].ID, "node1"; got != want {
		t.Fatalf("nodes[0].ID = %q, want %q", got, want)
	}
	if got, want := nodes[1].ID, "node2"; got != want {
		t.Fatalf("nodes[1].ID = %q, want %q", got, want)
	}

	leader, err := client1.FindLeader(context.Background())
	if err != nil {
		t.Fatalf("FindLeader() error = %v", err)
	}
	if got, want := leader.ID, "node1"; got != want {
		t.Fatalf("leader.ID = %q, want %q", got, want)
	}

	node2.IsLeader = true
	node2.HTTPAddr = "127.0.0.1:19081"
	if err := client2.Update(context.Background(), node2); err != nil {
		t.Fatalf("Update(node2) error = %v", err)
	}
	node1.IsLeader = false
	if err := client1.Update(context.Background(), node1); err != nil {
		t.Fatalf("Update(node1) error = %v", err)
	}

	waitForLeader(t, client1, "node2", 10*time.Second)

	if err := client1.MarkRemoved(context.Background(), "node3"); err != nil {
		t.Fatalf("MarkRemoved(node3) error = %v", err)
	}
	removed, err := client2.IsRemoved(context.Background(), "node3")
	if err != nil {
		t.Fatalf("IsRemoved(node3) error = %v", err)
	}
	if !removed {
		t.Fatalf("IsRemoved(node3) = false, want true")
	}
	removedIDs, err := client1.ListRemovedIDs(context.Background())
	if err != nil {
		t.Fatalf("ListRemovedIDs() error = %v", err)
	}
	if !reflect.DeepEqual(removedIDs, []string{"node3"}) {
		t.Fatalf("ListRemovedIDs() = %#v, want %#v", removedIDs, []string{"node3"})
	}
	if err := client2.UnmarkRemoved(context.Background(), "node3"); err != nil {
		t.Fatalf("UnmarkRemoved(node3) error = %v", err)
	}

	if err := client1.Close(); err != nil {
		t.Fatalf("client1.Close() error = %v", err)
	}
	waitForNodeCount(t, client2, 1, 12*time.Second)
	nodes, err = client2.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes(after close) error = %v", err)
	}
	if got, want := nodes[0].ID, "node2"; got != want {
		t.Fatalf("remaining node ID = %q, want %q", got, want)
	}
}

func TestEmbeddedClientSavesAndLoadsControllerConfig(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	client, err := New([]string{etcd.clientEndpoint})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = client.Close()
	}()

	config := shardmeta.ClusterConfig{
		Version:     3,
		TotalShards: 8,
		Groups: []shardmeta.GroupInfo{
			{ID: "g1"},
			{ID: "g2"},
		},
		Assignments: []shardmeta.ShardAssignment{
			{ShardID: 0, GroupID: "g1"},
			{ShardID: 1, GroupID: "g2"},
		},
	}
	if err := client.SaveControllerConfig(context.Background(), config); err != nil {
		t.Fatalf("SaveControllerConfig() error = %v", err)
	}

	loaded, err := client.LoadControllerConfig(context.Background())
	if err != nil {
		t.Fatalf("LoadControllerConfig() error = %v", err)
	}
	if !reflect.DeepEqual(loaded, config) {
		t.Fatalf("LoadControllerConfig() = %#v, want %#v", loaded, config)
	}
}

func waitForNodeCount(t *testing.T, client *Client, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		nodes, err := client.ListNodes(context.Background())
		if err == nil && len(nodes) == want {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}

	nodes, err := client.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	t.Fatalf("len(nodes) = %d, want %d", len(nodes), want)
}

func waitForLeader(t *testing.T, client *Client, wantID string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		leader, err := client.FindLeader(context.Background())
		if err == nil && leader != nil && leader.ID == wantID {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}

	leader, err := client.FindLeader(context.Background())
	if err != nil {
		t.Fatalf("FindLeader() error = %v", err)
	}
	t.Fatalf("leader.ID = %q, want %q", leader.ID, wantID)
}

type embeddedEtcd struct {
	server         *embedetcd.Etcd
	clientEndpoint string
}

func startEmbeddedEtcd(t *testing.T) *embeddedEtcd {
	t.Helper()

	clientAddr := reserveAddr(t)
	peerAddr := reserveAddr(t)
	clientURL, err := url.Parse("http://" + clientAddr)
	if err != nil {
		t.Fatalf("url.Parse(client) error = %v", err)
	}
	peerURL, err := url.Parse("http://" + peerAddr)
	if err != nil {
		t.Fatalf("url.Parse(peer) error = %v", err)
	}

	cfg := embedetcd.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.Name = "default"
	cfg.LogLevel = "error"
	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = []url.URL{*clientURL}
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = []url.URL{*peerURL}
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())

	server, err := embedetcd.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("StartEtcd() error = %v", err)
	}
	select {
	case <-server.Server.ReadyNotify():
	case <-time.After(15 * time.Second):
		server.Close()
		t.Fatalf("embedded etcd was not ready in time")
	}

	instance := &embeddedEtcd{
		server:         server,
		clientEndpoint: clientAddr,
	}
	t.Cleanup(func() {
		server.Close()
	})
	return instance
}

func reserveAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}
