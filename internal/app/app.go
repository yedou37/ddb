package app

import (
	"context"
	"net/http"
	"sync"
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
	stopCh    chan struct{}
	closeOnce sync.Once
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
	if discoveryClient != nil {
		removed, removeErr := discoveryClient.IsRemoved(context.Background(), cfg.NodeID)
		if removeErr != nil {
			_ = store.Close()
			_ = discoveryClient.Close()
			return nil, removeErr
		}
		if removed && !cfg.Rejoin {
			_ = store.Close()
			_ = discoveryClient.Close()
			return nil, http.ErrServerClosed
		}
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
		stopCh:    make(chan struct{}),
	}

	return app, nil
}

func (a *App) Run() error {
	joinAddr := a.cfg.JoinAddr
	if joinAddr == "" && !a.cfg.Bootstrap && a.discovery != nil {
		var err error
		joinAddr, err = a.discoverLeaderHTTP(context.Background())
		if err != nil {
			return err
		}
	}

	if a.cfg.Rejoin {
		if joinAddr == "" {
			return context.DeadlineExceeded
		}
		var err error
		for attempt := 0; attempt < 10; attempt++ {
			time.Sleep(500 * time.Millisecond)
			err = a.raftNode.RejoinCluster(context.Background(), joinAddr, a.cfg.NodeID, a.cfg.RaftAddr, a.cfg.HTTPAddr)
			if err == nil {
				break
			}
		}
		if err != nil {
			return err
		}
	} else if joinAddr != "" {
		var err error
		for attempt := 0; attempt < 10; attempt++ {
			time.Sleep(500 * time.Millisecond)
			err = a.raftNode.JoinCluster(context.Background(), joinAddr, a.cfg.NodeID, a.cfg.RaftAddr, a.cfg.HTTPAddr)
			if err == nil {
				break
			}
		}
		if err != nil {
			return err
		}
	}

	if a.discovery != nil {
		if err := a.discovery.Register(context.Background(), a.currentNodeInfo()); err != nil {
			return err
		}
	}

	go a.syncNodeState()
	return a.server.ListenAndServe()
}

func (a *App) Close() error {
	a.closeOnce.Do(func() {
		select {
		case <-a.stopCh:
		default:
			close(a.stopCh)
		}
		if a.server != nil {
			_ = a.server.Close()
		}
		if a.raftNode != nil {
			_ = a.raftNode.Close()
		}
		if a.discovery != nil {
			_ = a.discovery.Close()
		}
		if a.store != nil {
			_ = a.store.Close()
		}
	})
	return nil
}

func (a *App) syncNodeState() {
	if a.discovery == nil {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			removed, err := a.discovery.IsRemoved(context.Background(), a.cfg.NodeID)
			if err == nil && removed {
				_ = a.Close()
				return
			}
			_ = a.discovery.Update(context.Background(), a.currentNodeInfo())
		}
	}
}

func (a *App) currentNodeInfo() model.NodeInfo {
	return model.NodeInfo{
		ID:       a.cfg.NodeID,
		RaftAddr: a.cfg.RaftAddr,
		HTTPAddr: a.cfg.HTTPAddr,
		IsLeader: a.raftNode != nil && a.raftNode.IsLeader(),
	}
}

func (a *App) discoverLeaderHTTP(ctx context.Context) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 20; attempt++ {
		nodes, err := a.discovery.ListNodes(ctx)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, node := range nodes {
			if node.ID == a.cfg.NodeID || node.HTTPAddr == "" {
				continue
			}
			if node.IsLeader {
				return node.HTTPAddr, nil
			}
		}
		if len(nodes) > 0 {
			for _, node := range nodes {
				if node.ID != a.cfg.NodeID && node.HTTPAddr != "" {
					return node.HTTPAddr, nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", context.DeadlineExceeded
}
