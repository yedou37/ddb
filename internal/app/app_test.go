package app

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/model"
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
	if info != (model.NodeInfo{ID: "node1", RaftAddr: "127.0.0.1:7000", HTTPAddr: "127.0.0.1:8080"}) {
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
