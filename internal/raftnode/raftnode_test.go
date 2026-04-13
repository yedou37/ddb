package raftnode

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/raft"

	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/storage"
)

func TestDecodeApplyResponse(t *testing.T) {
	result, err := decodeApplyResponse(model.QueryResult{Type: "insert", RowsAffected: 1})
	if err != nil {
		t.Fatalf("decodeApplyResponse() error = %v", err)
	}
	if result.Type != "insert" {
		t.Fatalf("result.Type = %q, want insert", result.Type)
	}

	_, err = decodeApplyResponse(errors.New("boom"))
	if err == nil {
		t.Fatalf("decodeApplyResponse(error) = nil, want error")
	}
}

func TestNormalizeHTTPAddrAndRaftStateDir(t *testing.T) {
	if got, want := normalizeHTTPAddr("127.0.0.1:8080"), "http://127.0.0.1:8080"; got != want {
		t.Fatalf("normalizeHTTPAddr() = %q, want %q", got, want)
	}
	if got, want := raftStateDir("/tmp/raft", "node1"), filepath.Join("/tmp/raft", "node1"); got != want {
		t.Fatalf("raftStateDir() = %q, want %q", got, want)
	}
}

func TestFSMApply(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "fsm.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	fsm := newFSM(store)
	createResult := fsm.Apply(&raft.Log{Data: []byte("CREATE TABLE users (id INT PRIMARY KEY, name TEXT)")})
	if _, err := decodeApplyResponse(createResult); err != nil {
		t.Fatalf("fsm.Apply(create) error = %v", err)
	}

	insertResult := fsm.Apply(&raft.Log{Data: []byte("INSERT INTO users VALUES (1, 'alice')")})
	if _, err := decodeApplyResponse(insertResult); err != nil {
		t.Fatalf("fsm.Apply(insert) error = %v", err)
	}
}

func TestLeaderHTTPHint(t *testing.T) {
	node := &Node{}
	node.setLeaderHTTPHint("127.0.0.1:8080")

	if got, want := node.LeaderHTTPHint(), "http://127.0.0.1:8080"; got != want {
		t.Fatalf("LeaderHTTPHint() = %q, want %q", got, want)
	}
}

func TestCloseNilNode(t *testing.T) {
	node := &Node{}
	if err := node.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestJoinClusterSuccessAndFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/join"; got != want {
			t.Fatalf("r.URL.Path = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	node := &Node{}
	if err := node.JoinCluster(t.Context(), server.URL, "node2", "127.0.0.1:7001", "127.0.0.1:8081"); err != nil {
		t.Fatalf("JoinCluster() error = %v", err)
	}
	if got, want := node.LeaderHTTPHint(), server.URL; got != want {
		t.Fatalf("LeaderHTTPHint() = %q, want %q", got, want)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "join failed", http.StatusBadRequest)
	}))
	defer errorServer.Close()

	if err := node.JoinCluster(t.Context(), errorServer.URL, "node2", "127.0.0.1:7001", "127.0.0.1:8081"); err == nil {
		t.Fatalf("JoinCluster() error = nil, want error")
	}
}

func TestRejoinClusterSuccessAndFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/rejoin"; got != want {
			t.Fatalf("r.URL.Path = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	node := &Node{}
	if err := node.RejoinCluster(t.Context(), server.URL, "node2", "127.0.0.1:7001", "127.0.0.1:8081"); err != nil {
		t.Fatalf("RejoinCluster() error = %v", err)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rejoin failed", http.StatusBadRequest)
	}))
	defer errorServer.Close()

	if err := node.RejoinCluster(t.Context(), errorServer.URL, "node2", "127.0.0.1:7001", "127.0.0.1:8081"); err == nil {
		t.Fatalf("RejoinCluster() error = nil, want error")
	}
}

func TestNewBootstrapCloseAndReopen(t *testing.T) {
	baseDir := t.TempDir()
	dbPath := filepath.Join(baseDir, "node.db")
	raftDir := filepath.Join(baseDir, "raft")
	raftAddr := reserveRaftAddr(t)

	store, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}

	cfg := config.ServerConfig{
		NodeID:    "node1",
		HTTPAddr:  "127.0.0.1:8080",
		RaftAddr:  raftAddr,
		RaftDir:   raftDir,
		Bootstrap: true,
	}

	node, err := New(cfg, store)
	if err != nil {
		_ = store.Close()
		t.Fatalf("New() error = %v", err)
	}

	waitForLeader(t, node)

	if closeErr := node.Close(); closeErr != nil {
		t.Fatalf("first Close() error = %v", closeErr)
	}
	if closeErr := store.Close(); closeErr != nil {
		t.Fatalf("store.Close() error = %v", closeErr)
	}

	reopenedStore, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen storage.Open() error = %v", err)
	}
	defer func() {
		_ = reopenedStore.Close()
	}()

	reopenedNode, err := New(cfg, reopenedStore)
	if err != nil {
		t.Fatalf("reopen New() error = %v", err)
	}
	defer func() {
		_ = reopenedNode.Close()
	}()
}

func waitForLeader(t *testing.T, node *Node) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if node.IsLeader() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("node did not become leader before timeout")
}

func reserveRaftAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer listener.Close()
	return listener.Addr().String()
}
