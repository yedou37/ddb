package main

import (
	"errors"
	"net/http"
	"testing"

	"github.com/yedou37/ddb/internal/config"
)

func TestRunSuccessAndServerClosed(t *testing.T) {
	originalParse := parseServerConfig
	originalNew := newServerApp
	defer func() {
		parseServerConfig = originalParse
		newServerApp = originalNew
	}()

	parseServerConfig = func() config.ServerConfig {
		return config.ServerConfig{NodeID: "node1", HTTPAddr: "127.0.0.1:8080", RaftAddr: "127.0.0.1:7000"}
	}
	newServerApp = func(cfg config.ServerConfig) (serverApp, error) {
		return &fakeServerApp{runErr: http.ErrServerClosed}, nil
	}

	if err := run(); err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
}

func TestRunReturnsAppError(t *testing.T) {
	originalParse := parseServerConfig
	originalNew := newServerApp
	defer func() {
		parseServerConfig = originalParse
		newServerApp = originalNew
	}()

	parseServerConfig = func() config.ServerConfig {
		return config.ServerConfig{}
	}
	newServerApp = func(cfg config.ServerConfig) (serverApp, error) {
		return nil, errors.New("boom")
	}

	if err := run(); err == nil {
		t.Fatalf("run() error = nil, want error")
	}
}

type fakeServerApp struct {
	runErr error
}

func (f *fakeServerApp) Run() error   { return f.runErr }
func (f *fakeServerApp) Close() error { return nil }
