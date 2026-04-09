package app

import (
	"context"
	"net/http"
	"time"

	"github.com/yedou37/dbd/internal/api"
	"github.com/yedou37/dbd/internal/config"
	"github.com/yedou37/dbd/internal/discovery"
	"github.com/yedou37/dbd/internal/model"
	"github.com/yedou37/dbd/internal/raftnode"
	"github.com/yedou37/dbd/internal/service"
	"github.com/yedou37/dbd/internal/storage"
)

type App struct {
	server    *http.Server
	raftNode  *raftnode.Node
	store     *storage.Store
	discovery *discovery.Client
	cfg       config.ServerConfig
}

func NewServerApp(cfg config.ServerConfig) (*App, error) {
	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	discoveryClient, err := discovery.New(cfg.ETCDEndpoints)
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	raftNode, err := raftnode.New(cfg, store)
	if err != nil {
		_ = store.Close()
		if discoveryClient != nil {
			_ = discoveryClient.Close()
		}
		return nil, err
	}

	queryService := service.NewQueryService(cfg.NodeID, cfg.HTTPAddr, cfg.RaftAddr, store, raftNode, discoveryClient)
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: api.NewHandler(queryService),
	}

	app := &App{
		server:    httpServer,
		raftNode:  raftNode,
		store:     store,
		discovery: discoveryClient,
		cfg:       cfg,
	}

	if discoveryClient != nil {
		err = discoveryClient.Register(context.Background(), model.NodeInfo{
			ID:       cfg.NodeID,
			RaftAddr: cfg.RaftAddr,
			HTTPAddr: cfg.HTTPAddr,
			IsLeader: cfg.Bootstrap,
		})
		if err != nil {
			_ = app.Close()
			return nil, err
		}
	}

	return app, nil
}

func (a *App) Run() error {
	if a.cfg.JoinAddr != "" {
		var err error
		for attempt := 0; attempt < 10; attempt++ {
			time.Sleep(500 * time.Millisecond)
			err = a.raftNode.JoinCluster(context.Background(), a.cfg.JoinAddr, a.cfg.NodeID, a.cfg.RaftAddr, a.cfg.HTTPAddr)
			if err == nil {
				break
			}
		}
		if err != nil {
			return err
		}
	}
	return a.server.ListenAndServe()
}

func (a *App) Close() error {
	if a.raftNode != nil {
		_ = a.raftNode.Close()
	}
	if a.discovery != nil {
		_ = a.discovery.Close()
	}
	if a.store != nil {
		_ = a.store.Close()
	}
	return nil
}
