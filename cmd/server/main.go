package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/yedou37/dbd/internal/app"
	"github.com/yedou37/dbd/internal/config"
)

func main() {
	cfg := config.ParseServerConfig()

	application, err := app.NewServerApp(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = application.Close()
	}()

	log.Printf("node=%s http=%s raft=%s bootstrap=%t join=%s", cfg.NodeID, cfg.HTTPAddr, cfg.RaftAddr, cfg.Bootstrap, cfg.JoinAddr)

	err = application.Run()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
