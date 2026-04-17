package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/shardmeta"
)

func TestCurrentNodeInfo(t *testing.T) {
	app := &App{
		cfg: config.ServerConfig{
			NodeID:   "node1",
			HTTPAddr: "127.0.0.1:8080",
			RaftAddr: "127.0.0.1:7000",
		},
	}

	info := app.currentNodeInfo()
	if info != (model.NodeInfo{ID: "node1", RaftAddr: "127.0.0.1:7000", HTTPAddr: "127.0.0.1:8080", Role: "shard"}) {
		t.Fatalf("currentNodeInfo() = %#v", info)
	}
}

func TestCloseIdempotent(t *testing.T) {
	app := &App{stopCh: make(chan struct{})}
	if err := app.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestRunRejoinWithoutJoinAddress(t *testing.T) {
	app := &App{
		cfg: config.ServerConfig{
			NodeID: "node1",
			Rejoin: true,
		},
		stopCh: make(chan struct{}),
	}

	err := app.Run()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestRunReturnsListenError(t *testing.T) {
	app := &App{
		cfg:    config.ServerConfig{NodeID: "node1", Bootstrap: true},
		server: &http.Server{Addr: "127.0.0.1:-1"},
		stopCh: make(chan struct{}),
	}

	if err := app.Run(); err == nil {
		t.Fatalf("Run() error = nil, want listen error")
	}
}

func TestNewServerAppControllerRoleUsesAPIHandler(t *testing.T) {
	app, err := NewServerApp(config.ServerConfig{
		NodeID:   "controller-1",
		HTTPAddr: "127.0.0.1:18080",
		Role:     shardmeta.RoleController,
	})
	if err != nil {
		t.Fatalf("NewServerApp(controller) error = %v", err)
	}
	defer func() {
		_ = app.Close()
	}()

	if app.raftNode != nil {
		t.Fatalf("app.raftNode != nil for controller role")
	}
	if app.controllerService == nil {
		t.Fatalf("app.controllerService = nil, want non-nil")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	app.server.Handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("/config code = %d, want %d", got, want)
	}
}

func TestNewServerAppAPIServerRoleUsesAPIHandler(t *testing.T) {
	app, err := NewServerApp(config.ServerConfig{
		NodeID:   "api-1",
		HTTPAddr: "127.0.0.1:18081",
		Role:     shardmeta.RoleAPIServer,
	})
	if err != nil {
		t.Fatalf("NewServerApp(apiserver) error = %v", err)
	}
	defer func() {
		_ = app.Close()
	}()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	app.server.Handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("/health code = %d, want %d", got, want)
	}
}

func TestPickBootstrapGroupsDefaultsToG1G2(t *testing.T) {
	groups := pickBootstrapGroups(nil)
	if got, want := len(groups), 2; got != want {
		t.Fatalf("len(groups) = %d, want %d", got, want)
	}
	if got, want := groups[0], shardmeta.GroupID("g1"); got != want {
		t.Fatalf("groups[0] = %q, want %q", got, want)
	}
	if got, want := groups[1], shardmeta.GroupID("g2"); got != want {
		t.Fatalf("groups[1] = %q, want %q", got, want)
	}
}

func TestPickBootstrapGroupsPrefersDetectedG1G2(t *testing.T) {
	groups := pickBootstrapGroups([]model.NodeInfo{
		{ID: "g3-n1", Role: "shard", GroupID: "g3"},
		{ID: "g1-n1", Role: "shard", GroupID: "g1"},
		{ID: "g2-n1", Role: "shard", GroupID: "g2"},
	})
	if got, want := len(groups), 2; got != want {
		t.Fatalf("len(groups) = %d, want %d", got, want)
	}
	if got, want := groups[0], shardmeta.GroupID("g1"); got != want {
		t.Fatalf("groups[0] = %q, want %q", got, want)
	}
	if got, want := groups[1], shardmeta.GroupID("g2"); got != want {
		t.Fatalf("groups[1] = %q, want %q", got, want)
	}
}

func TestPickBootstrapGroupsFallsBackToDiscoveredGroups(t *testing.T) {
	groups := pickBootstrapGroups([]model.NodeInfo{
		{ID: "a-n1", Role: "shard", GroupID: "alpha"},
		{ID: "b-n1", Role: "shard", GroupID: "beta"},
		{ID: "c-n1", Role: "shard", GroupID: "gamma"},
	})
	if got, want := len(groups), 2; got != want {
		t.Fatalf("len(groups) = %d, want %d", got, want)
	}
	if got, want := groups[0], shardmeta.GroupID("alpha"); got != want {
		t.Fatalf("groups[0] = %q, want %q", got, want)
	}
	if got, want := groups[1], shardmeta.GroupID("beta"); got != want {
		t.Fatalf("groups[1] = %q, want %q", got, want)
	}
}
