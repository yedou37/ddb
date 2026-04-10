package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
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

// #region debug-point A:app-lifecycle
func reportAppDebugEvent(hypothesisID, location, msg string, data map[string]any) {
	serverURL := "http://127.0.0.1:7777/event"
	sessionID := "ci-node3-restart"
	if content, err := os.ReadFile(".dbg/ci-node3-restart.env"); err == nil {
		for _, line := range strings.Split(string(content), "\n") {
			if value, ok := strings.CutPrefix(line, "DEBUG_SERVER_URL="); ok {
				serverURL = value
			}
			if value, ok := strings.CutPrefix(line, "DEBUG_SESSION_ID="); ok {
				sessionID = value
			}
		}
	}

	payload, err := json.Marshal(map[string]any{
		"sessionId":    sessionID,
		"runId":        "pre-fix",
		"hypothesisId": hypothesisID,
		"location":     location,
		"msg":          msg,
		"data":         data,
		"ts":           time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}

	go func(url string, body []byte) {
		request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err == nil && response != nil {
			_ = response.Body.Close()
		}
	}(serverURL, payload)
}

// #endregion

func NewServerApp(cfg config.ServerConfig) (*App, error) {
	reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:begin", "[DEBUG] NewServerApp begin", map[string]any{
		"nodeID":    cfg.NodeID,
		"httpAddr":  cfg.HTTPAddr,
		"raftAddr":  cfg.RaftAddr,
		"joinAddr":  cfg.JoinAddr,
		"bootstrap": cfg.Bootstrap,
		"rejoin":    cfg.Rejoin,
	})
	reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:store-begin", "[DEBUG] storage.Open begin", map[string]any{
		"nodeID": cfg.NodeID,
		"dbPath": cfg.DBPath,
	})
	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:store-error", "[DEBUG] storage.Open failed", map[string]any{
			"nodeID": cfg.NodeID,
			"error":  err.Error(),
		})
		return nil, err
	}
	reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:store-ok", "[DEBUG] storage.Open succeeded", map[string]any{
		"nodeID": cfg.NodeID,
		"dbPath": cfg.DBPath,
	})

	reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:discovery-begin", "[DEBUG] discovery.New begin", map[string]any{
		"nodeID": cfg.NodeID,
	})
	discoveryClient, err := discovery.New(cfg.ETCDEndpoints)
	if err != nil {
		reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:discovery-error", "[DEBUG] discovery.New failed", map[string]any{
			"nodeID": cfg.NodeID,
			"error":  err.Error(),
		})
		_ = store.Close()
		return nil, err
	}
	reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:discovery-ok", "[DEBUG] discovery.New succeeded", map[string]any{
		"nodeID": cfg.NodeID,
	})
	if discoveryClient != nil {
		removed, removeErr := discoveryClient.IsRemoved(context.Background(), cfg.NodeID)
		if removeErr != nil {
			reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:is-removed-error", "[DEBUG] discovery.IsRemoved failed", map[string]any{
				"nodeID": cfg.NodeID,
				"error":  removeErr.Error(),
			})
			_ = store.Close()
			_ = discoveryClient.Close()
			return nil, removeErr
		}
		if removed && !cfg.Rejoin {
			reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:removed", "[DEBUG] node marked removed during startup", map[string]any{
				"nodeID": cfg.NodeID,
			})
			_ = store.Close()
			_ = discoveryClient.Close()
			return nil, http.ErrServerClosed
		}
	}

	reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:raft-begin", "[DEBUG] raftnode.New begin", map[string]any{
		"nodeID": cfg.NodeID,
	})
	raftNode, err := raftnode.New(cfg, store)
	if err != nil {
		reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:raft-error", "[DEBUG] raftnode.New failed", map[string]any{
			"nodeID": cfg.NodeID,
			"error":  err.Error(),
		})
		_ = store.Close()
		if discoveryClient != nil {
			_ = discoveryClient.Close()
		}
		return nil, err
	}
	reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:raft-ok", "[DEBUG] raftnode.New succeeded", map[string]any{
		"nodeID": cfg.NodeID,
	})
	reportAppDebugEvent("B", "internal/app/app.go:NewServerApp:ready", "[DEBUG] NewServerApp ready", map[string]any{
		"nodeID": cfg.NodeID,
	})

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
	reportAppDebugEvent("A", "internal/app/app.go:Run:begin", "[DEBUG] App.Run begin", map[string]any{
		"nodeID":    a.cfg.NodeID,
		"joinAddr":  a.cfg.JoinAddr,
		"bootstrap": a.cfg.Bootstrap,
		"rejoin":    a.cfg.Rejoin,
	})
	joinAddr := a.cfg.JoinAddr
	if joinAddr == "" && !a.cfg.Bootstrap && a.discovery != nil {
		reportAppDebugEvent("C", "internal/app/app.go:Run:discover-leader-begin", "[DEBUG] discoverLeaderHTTP begin", map[string]any{
			"nodeID": a.cfg.NodeID,
		})
		var err error
		joinAddr, err = a.discoverLeaderHTTP(context.Background())
		if err != nil {
			reportAppDebugEvent("C", "internal/app/app.go:Run:discover-leader-error", "[DEBUG] discoverLeaderHTTP failed", map[string]any{
				"nodeID": a.cfg.NodeID,
				"error":  err.Error(),
			})
			return err
		}
		reportAppDebugEvent("C", "internal/app/app.go:Run:discover-leader-ok", "[DEBUG] discoverLeaderHTTP succeeded", map[string]any{
			"nodeID":   a.cfg.NodeID,
			"joinAddr": joinAddr,
		})
	}

	if a.cfg.Rejoin {
		if joinAddr == "" {
			reportAppDebugEvent("C", "internal/app/app.go:Run:rejoin-missing-joinaddr", "[DEBUG] rejoin missing joinAddr", map[string]any{
				"nodeID": a.cfg.NodeID,
			})
			return context.DeadlineExceeded
		}
		var err error
		for attempt := 0; attempt < 10; attempt++ {
			time.Sleep(500 * time.Millisecond)
			reportAppDebugEvent("C", "internal/app/app.go:Run:rejoin-attempt", "[DEBUG] rejoin attempt", map[string]any{
				"nodeID":   a.cfg.NodeID,
				"attempt":  attempt + 1,
				"joinAddr": joinAddr,
			})
			err = a.raftNode.RejoinCluster(context.Background(), joinAddr, a.cfg.NodeID, a.cfg.RaftAddr, a.cfg.HTTPAddr)
			if err == nil {
				reportAppDebugEvent("C", "internal/app/app.go:Run:rejoin-ok", "[DEBUG] rejoin succeeded", map[string]any{
					"nodeID":  a.cfg.NodeID,
					"attempt": attempt + 1,
				})
				break
			}
		}
		if err != nil {
			reportAppDebugEvent("C", "internal/app/app.go:Run:rejoin-error", "[DEBUG] rejoin failed", map[string]any{
				"nodeID": a.cfg.NodeID,
				"error":  err.Error(),
			})
			return err
		}
	} else if joinAddr != "" {
		var err error
		for attempt := 0; attempt < 10; attempt++ {
			time.Sleep(500 * time.Millisecond)
			reportAppDebugEvent("C", "internal/app/app.go:Run:join-attempt", "[DEBUG] join attempt", map[string]any{
				"nodeID":   a.cfg.NodeID,
				"attempt":  attempt + 1,
				"joinAddr": joinAddr,
			})
			err = a.raftNode.JoinCluster(context.Background(), joinAddr, a.cfg.NodeID, a.cfg.RaftAddr, a.cfg.HTTPAddr)
			if err == nil {
				reportAppDebugEvent("C", "internal/app/app.go:Run:join-ok", "[DEBUG] join succeeded", map[string]any{
					"nodeID":  a.cfg.NodeID,
					"attempt": attempt + 1,
				})
				break
			}
		}
		if err != nil {
			reportAppDebugEvent("C", "internal/app/app.go:Run:join-error", "[DEBUG] join failed", map[string]any{
				"nodeID": a.cfg.NodeID,
				"error":  err.Error(),
			})
			return err
		}
	}

	if a.discovery != nil {
		reportAppDebugEvent("C", "internal/app/app.go:Run:register-begin", "[DEBUG] discovery register begin", map[string]any{
			"nodeID": a.cfg.NodeID,
		})
		if err := a.discovery.Register(context.Background(), a.currentNodeInfo()); err != nil {
			reportAppDebugEvent("C", "internal/app/app.go:Run:register-error", "[DEBUG] discovery register failed", map[string]any{
				"nodeID": a.cfg.NodeID,
				"error":  err.Error(),
			})
			return err
		}
		reportAppDebugEvent("C", "internal/app/app.go:Run:register-ok", "[DEBUG] discovery register succeeded", map[string]any{
			"nodeID": a.cfg.NodeID,
		})
	}

	go a.syncNodeState()
	reportAppDebugEvent("D", "internal/app/app.go:Run:listen-begin", "[DEBUG] http listen begin", map[string]any{
		"nodeID":   a.cfg.NodeID,
		"httpAddr": a.cfg.HTTPAddr,
	})
	err := a.server.ListenAndServe()
	errData := map[string]any{
		"nodeID": a.cfg.NodeID,
	}
	if err != nil {
		errData["error"] = err.Error()
	}
	reportAppDebugEvent("D", "internal/app/app.go:Run:listen-exit", "[DEBUG] http listen exit", errData)
	return err
}

func (a *App) Close() error {
	a.closeOnce.Do(func() {
		reportAppDebugEvent("E", "internal/app/app.go:Close:begin", "[DEBUG] App.Close begin", map[string]any{
			"nodeID": a.cfg.NodeID,
		})
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
		reportAppDebugEvent("E", "internal/app/app.go:Close:done", "[DEBUG] App.Close done", map[string]any{
			"nodeID": a.cfg.NodeID,
		})
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
