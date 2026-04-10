package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/yedou37/dbd/internal/app"
	"github.com/yedou37/dbd/internal/config"
)

type serverApp interface {
	Run() error
	Close() error
}

var (
	parseServerConfig = config.ParseServerConfig
	newServerApp      = func(cfg config.ServerConfig) (serverApp, error) {
		return app.NewServerApp(cfg)
	}
)

func run() error {
	cfg := parseServerConfig()

	application, err := newServerApp(cfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = application.Close()
	}()

	log.Printf("node=%s http=%s raft=%s bootstrap=%t join=%s", cfg.NodeID, cfg.HTTPAddr, cfg.RaftAddr, cfg.Bootstrap, cfg.JoinAddr)

	err = application.Run()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
