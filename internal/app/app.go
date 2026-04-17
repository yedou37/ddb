package app

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/yedou37/ddb/internal/api"
	"github.com/yedou37/ddb/internal/apiserver"
	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/controller"
	"github.com/yedou37/ddb/internal/coordinator"
	"github.com/yedou37/ddb/internal/discovery"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/raftnode"
	"github.com/yedou37/ddb/internal/router"
	"github.com/yedou37/ddb/internal/service"
	"github.com/yedou37/ddb/internal/shardmeta"
	"github.com/yedou37/ddb/internal/storage"
)

type App struct {
	server            *http.Server
	raftNode          *raftnode.Node
	store             *storage.Store
	discovery         *discovery.Client
	controllerService *controller.Service
	cfg               config.ServerConfig
	stopCh            chan struct{}
	closeOnce         sync.Once
}

func NewServerApp(cfg config.ServerConfig) (*App, error) {
	discoveryClient, err := discovery.New(cfg.ETCDEndpoints)
	if err != nil {
		return nil, err
	}
	role := cfg.Role.OrDefault()
	if discoveryClient != nil && role == shardmeta.RoleShardNode {
		removed, removeErr := discoveryClient.IsRemoved(context.Background(), cfg.NodeID)
		if removeErr != nil {
			_ = discoveryClient.Close()
			return nil, removeErr
		}
		if removed && !cfg.Rejoin {
			_ = discoveryClient.Close()
			return nil, http.ErrServerClosed
		}
	}

	app := &App{
		discovery: discoveryClient,
		cfg:       cfg,
		stopCh:    make(chan struct{}),
	}

	switch role {
	case shardmeta.RoleController, shardmeta.RoleAPIServer:
		bootstrapGroups, discoverErr := discoverBootstrapGroups(context.Background(), discoveryClient)
		if discoverErr != nil {
			if discoveryClient != nil {
				_ = discoveryClient.Close()
			}
			return nil, discoverErr
		}
		var fileStore controller.ConfigStore
		if cfg.DBPath != "" {
			fileStore = controller.NewFileStore(cfg.DBPath + ".controller.json")
		}
		configStore := controller.NewChainStore(
			controller.NewDiscoveryStore(discoveryClient),
			fileStore,
		)
		controllerService, err := controller.NewBootstrapService(
			shardmeta.DefaultTotalShards,
			bootstrapGroups,
			configStore,
		)
		if err != nil {
			if discoveryClient != nil {
				_ = discoveryClient.Close()
			}
			return nil, err
		}
		app.controllerService = controllerService
		routeEngine, routeErr := router.New(shardmeta.DefaultTotalShards)
		if routeErr != nil {
			if discoveryClient != nil {
				_ = discoveryClient.Close()
			}
			return nil, routeErr
		}
		sqlCoordinator := coordinator.New(controllerService, discoveryClient, routeEngine)
		app.server = &http.Server{
			Addr:    cfg.HTTPAddr,
			Handler: apiserver.NewHandler(controllerService, discoveryClient, sqlCoordinator, sqlCoordinator),
		}
	default:
		store, openErr := storage.Open(cfg.DBPath)
		if openErr != nil {
			if discoveryClient != nil {
				_ = discoveryClient.Close()
			}
			return nil, openErr
		}
		raftNode, raftErr := raftnode.New(cfg, store)
		if raftErr != nil {
			_ = store.Close()
			if discoveryClient != nil {
				_ = discoveryClient.Close()
			}
			return nil, raftErr
		}

		queryService := service.NewQueryService(cfg.NodeID, cfg.HTTPAddr, cfg.RaftAddr, store, raftNode, discoveryClient)
		app.store = store
		app.raftNode = raftNode
		app.server = &http.Server{
			Addr:    cfg.HTTPAddr,
			Handler: api.NewHandler(queryService),
		}
	}

	return app, nil
}

func (a *App) Run() error {
	if a.cfg.Role.OrDefault() != shardmeta.RoleShardNode {
		if a.discovery != nil {
			if err := a.discovery.Register(context.Background(), a.currentNodeInfo()); err != nil {
				return err
			}
		}
		go a.syncNodeState()
		return a.server.ListenAndServe()
	}

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
			if a.cfg.Role.OrDefault() == shardmeta.RoleShardNode {
				removed, err := a.discovery.IsRemoved(context.Background(), a.cfg.NodeID)
				if err == nil && removed {
					_ = a.Close()
					return
				}
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
		Role:     string(a.cfg.Role.OrDefault()),
		GroupID:  a.cfg.GroupID,
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

func discoverBootstrapGroups(ctx context.Context, discoveryClient *discovery.Client) ([]shardmeta.GroupID, error) {
	defaultGroups := []shardmeta.GroupID{"g1", "g2"}
	if discoveryClient == nil {
		return defaultGroups, nil
	}

	nodes, err := discoveryClient.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	return pickBootstrapGroups(nodes), nil
}

func pickBootstrapGroups(nodes []model.NodeInfo) []shardmeta.GroupID {
	preferred := []string{"g1", "g2"}
	seen := make(map[string]bool)
	discovered := make([]string, 0)
	for _, node := range nodes {
		if node.Role != "" && node.Role != string(shardmeta.RoleShardNode) {
			continue
		}
		if node.GroupID == "" || seen[node.GroupID] {
			continue
		}
		seen[node.GroupID] = true
		discovered = append(discovered, node.GroupID)
	}
	slices.Sort(discovered)

	result := make([]shardmeta.GroupID, 0, 2)
	for _, groupID := range preferred {
		if seen[groupID] {
			result = append(result, shardmeta.GroupID(groupID))
		}
	}
	if len(result) >= 2 {
		return result[:2]
	}
	for _, groupID := range discovered {
		if slices.ContainsFunc(result, func(value shardmeta.GroupID) bool { return string(value) == groupID }) {
			continue
		}
		result = append(result, shardmeta.GroupID(groupID))
		if len(result) == 2 {
			return result
		}
	}
	if len(result) == 0 {
		return []shardmeta.GroupID{"g1", "g2"}
	}
	return result
}
