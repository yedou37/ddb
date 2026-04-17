package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yedou37/ddb/internal/controller"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/router"
	"github.com/yedou37/ddb/internal/shardmeta"
)

type stubNodeLister struct {
	nodes []model.NodeInfo
}

func (s stubNodeLister) ListNodes(context.Context) ([]model.NodeInfo, error) {
	return s.nodes, nil
}

func TestCoordinatorRoutesSingleShardSQL(t *testing.T) {
	routeEngine, err := router.New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("router.New() error = %v", err)
	}

	config, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("controller.NewService() error = %v", err)
	}

	routeResult, err := routeEngine.Route("users", 42, config.CurrentConfig())
	if err != nil {
		t.Fatalf("routeEngine.Route() error = %v", err)
	}

	var hitG1, hitG2 int
	serverG1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/schema" {
			_ = json.NewEncoder(w).Encode(model.TableSchema{
				Name:       "users",
				PrimaryKey: "id",
				Columns: []model.ColumnDef{
					{Name: "id", Type: "INT", PrimaryKey: true},
					{Name: "name", Type: "TEXT"},
				},
			})
			return
		}
		hitG1++
		_ = json.NewEncoder(w).Encode(model.SQLResponse{Success: true, Result: model.QueryResult{Type: "select"}})
	}))
	defer serverG1.Close()
	serverG2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/schema" {
			_ = json.NewEncoder(w).Encode(model.TableSchema{
				Name:       "users",
				PrimaryKey: "id",
				Columns: []model.ColumnDef{
					{Name: "id", Type: "INT", PrimaryKey: true},
					{Name: "name", Type: "TEXT"},
				},
			})
			return
		}
		hitG2++
		_ = json.NewEncoder(w).Encode(model.SQLResponse{Success: true, Result: model.QueryResult{Type: "select"}})
	}))
	defer serverG2.Close()

	instance := New(config, stubNodeLister{nodes: []model.NodeInfo{
		{ID: "g1-n1", HTTPAddr: serverG1.URL, Role: string(shardmeta.RoleShardNode), GroupID: "g1", IsLeader: true},
		{ID: "g2-n1", HTTPAddr: serverG2.URL, Role: string(shardmeta.RoleShardNode), GroupID: "g2", IsLeader: true},
	}}, routeEngine)

	response, err := instance.ExecuteSQL(context.Background(), "SELECT * FROM users WHERE id = 42")
	if err != nil {
		t.Fatalf("ExecuteSQL() error = %v", err)
	}
	if !response.Success {
		t.Fatalf("response.Success = false, want true")
	}

	switch routeResult.GroupID {
	case "g1":
		if hitG1 != 1 || hitG2 != 0 {
			t.Fatalf("hits = g1:%d g2:%d, want g1:1 g2:0", hitG1, hitG2)
		}
	case "g2":
		if hitG1 != 0 || hitG2 != 1 {
			t.Fatalf("hits = g1:%d g2:%d, want g1:0 g2:1", hitG1, hitG2)
		}
	default:
		t.Fatalf("unexpected routeResult.GroupID = %q", routeResult.GroupID)
	}
}

func TestCoordinatorBroadcastsCreateTable(t *testing.T) {
	routeEngine, err := router.New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("router.New() error = %v", err)
	}

	config, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("controller.NewService() error = %v", err)
	}

	var hitG1, hitG2 int
	serverG1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitG1++
		_ = json.NewEncoder(w).Encode(model.SQLResponse{Success: true, Result: model.QueryResult{Type: "create_table"}})
	}))
	defer serverG1.Close()
	serverG2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitG2++
		_ = json.NewEncoder(w).Encode(model.SQLResponse{Success: true, Result: model.QueryResult{Type: "create_table"}})
	}))
	defer serverG2.Close()

	instance := New(config, stubNodeLister{nodes: []model.NodeInfo{
		{ID: "g1-n1", HTTPAddr: serverG1.URL, Role: string(shardmeta.RoleShardNode), GroupID: "g1", IsLeader: true},
		{ID: "g2-n1", HTTPAddr: serverG2.URL, Role: string(shardmeta.RoleShardNode), GroupID: "g2", IsLeader: true},
	}}, routeEngine)

	response, err := instance.ExecuteSQL(context.Background(), "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("ExecuteSQL() error = %v", err)
	}
	if !response.Success {
		t.Fatalf("response.Success = false, want true")
	}
	if hitG1 != 1 || hitG2 != 1 {
		t.Fatalf("hits = g1:%d g2:%d, want g1:1 g2:1", hitG1, hitG2)
	}
}

func TestCoordinatorScatterSelectAcrossGroups(t *testing.T) {
	routeEngine, err := router.New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("router.New() error = %v", err)
	}

	config, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("controller.NewService() error = %v", err)
	}

	serverG1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(model.SQLResponse{
			Success: true,
			Result: model.QueryResult{
				Type:    "select",
				Columns: []string{"id", "name"},
				Rows:    [][]any{{1, "alice"}},
			},
		})
	}))
	defer serverG1.Close()
	serverG2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(model.SQLResponse{
			Success: true,
			Result: model.QueryResult{
				Type:    "select",
				Columns: []string{"id", "name"},
				Rows:    [][]any{{2, "bob"}},
			},
		})
	}))
	defer serverG2.Close()

	instance := New(config, stubNodeLister{nodes: []model.NodeInfo{
		{ID: "g1-n1", HTTPAddr: serverG1.URL, Role: string(shardmeta.RoleShardNode), GroupID: "g1", IsLeader: true},
		{ID: "g2-n1", HTTPAddr: serverG2.URL, Role: string(shardmeta.RoleShardNode), GroupID: "g2", IsLeader: true},
	}}, routeEngine)

	response, err := instance.ExecuteSQL(context.Background(), "SELECT * FROM users")
	if err != nil {
		t.Fatalf("ExecuteSQL() error = %v", err)
	}
	if !response.Success {
		t.Fatalf("response.Success = false, want true")
	}
	if got, want := len(response.Result.Rows), 2; got != want {
		t.Fatalf("len(response.Result.Rows) = %d, want %d", got, want)
	}
}

func TestCoordinatorExecutesEqualityJoin(t *testing.T) {
	routeEngine, err := router.New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("router.New() error = %v", err)
	}

	config, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("controller.NewService() error = %v", err)
	}

	handlerG1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/schema" && r.URL.Query().Get("table") == "users":
			_ = json.NewEncoder(w).Encode(model.TableSchema{
				Name:       "users",
				PrimaryKey: "id",
				Columns: []model.ColumnDef{
					{Name: "id", Type: "INT", PrimaryKey: true},
					{Name: "name", Type: "TEXT"},
				},
			})
		case r.URL.Path == "/schema" && r.URL.Query().Get("table") == "orders":
			_ = json.NewEncoder(w).Encode(model.TableSchema{
				Name:       "orders",
				PrimaryKey: "id",
				Columns: []model.ColumnDef{
					{Name: "id", Type: "INT", PrimaryKey: true},
					{Name: "user_id", Type: "INT"},
					{Name: "item", Type: "TEXT"},
				},
			})
		default:
			var request model.SQLRequest
			if decodeErr := json.NewDecoder(r.Body).Decode(&request); decodeErr != nil {
				t.Fatalf("json.Decode(SQLRequest) error = %v", decodeErr)
			}
			switch request.SQL {
			case "SELECT * FROM users":
				_ = json.NewEncoder(w).Encode(model.SQLResponse{
					Success: true,
					Result: model.QueryResult{
						Type:    "select",
						Columns: []string{"id", "name"},
						Rows:    [][]any{{1, "alice"}},
					},
				})
			case "SELECT * FROM orders":
				_ = json.NewEncoder(w).Encode(model.SQLResponse{
					Success: true,
					Result: model.QueryResult{
						Type:    "select",
						Columns: []string{"id", "user_id", "item"},
						Rows:    [][]any{},
					},
				})
			default:
				t.Fatalf("unexpected SQL: %s", request.SQL)
			}
		}
	})
	handlerG2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/schema" && r.URL.Query().Get("table") == "users":
			_ = json.NewEncoder(w).Encode(model.TableSchema{
				Name:       "users",
				PrimaryKey: "id",
				Columns: []model.ColumnDef{
					{Name: "id", Type: "INT", PrimaryKey: true},
					{Name: "name", Type: "TEXT"},
				},
			})
		case r.URL.Path == "/schema" && r.URL.Query().Get("table") == "orders":
			_ = json.NewEncoder(w).Encode(model.TableSchema{
				Name:       "orders",
				PrimaryKey: "id",
				Columns: []model.ColumnDef{
					{Name: "id", Type: "INT", PrimaryKey: true},
					{Name: "user_id", Type: "INT"},
					{Name: "item", Type: "TEXT"},
				},
			})
		default:
			var request model.SQLRequest
			if decodeErr := json.NewDecoder(r.Body).Decode(&request); decodeErr != nil {
				t.Fatalf("json.Decode(SQLRequest) error = %v", decodeErr)
			}
			switch request.SQL {
			case "SELECT * FROM users":
				_ = json.NewEncoder(w).Encode(model.SQLResponse{
					Success: true,
					Result: model.QueryResult{
						Type:    "select",
						Columns: []string{"id", "name"},
						Rows:    [][]any{},
					},
				})
			case "SELECT * FROM orders":
				_ = json.NewEncoder(w).Encode(model.SQLResponse{
					Success: true,
					Result: model.QueryResult{
						Type:    "select",
						Columns: []string{"id", "user_id", "item"},
						Rows:    [][]any{{10, 1, "book"}},
					},
				})
			default:
				t.Fatalf("unexpected SQL: %s", request.SQL)
			}
		}
	})
	serverG1 := httptest.NewServer(handlerG1)
	defer serverG1.Close()
	serverG2 := httptest.NewServer(handlerG2)
	defer serverG2.Close()

	instance := New(config, stubNodeLister{nodes: []model.NodeInfo{
		{ID: "g1-n1", HTTPAddr: serverG1.URL, Role: string(shardmeta.RoleShardNode), GroupID: "g1", IsLeader: true},
		{ID: "g2-n1", HTTPAddr: serverG2.URL, Role: string(shardmeta.RoleShardNode), GroupID: "g2", IsLeader: true},
	}}, routeEngine)

	response, err := instance.ExecuteSQL(context.Background(), "SELECT * FROM users JOIN orders ON users.id = orders.user_id")
	if err != nil {
		t.Fatalf("ExecuteSQL() error = %v", err)
	}
	if !response.Success {
		t.Fatalf("response.Success = false, want true")
	}
	if got, want := response.Result.Type, "join"; got != want {
		t.Fatalf("response.Result.Type = %q, want %q", got, want)
	}
	if got, want := len(response.Result.Rows), 1; got != want {
		t.Fatalf("len(response.Result.Rows) = %d, want %d", got, want)
	}
	if got, want := response.Result.Columns[0], "users.id"; got != want {
		t.Fatalf("response.Result.Columns[0] = %q, want %q", got, want)
	}
}

func TestCoordinatorRejectsLockedShardRequests(t *testing.T) {
	routeEngine, err := router.New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("router.New() error = %v", err)
	}

	config, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("controller.NewService() error = %v", err)
	}

	routeResult, err := routeEngine.Route("users", 42, config.CurrentConfig())
	if err != nil {
		t.Fatalf("routeEngine.Route() error = %v", err)
	}
	if lockErr := config.LockShards(routeResult.ShardID); lockErr != nil {
		t.Fatalf("config.LockShards() error = %v", lockErr)
	}

	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/schema" {
			_ = json.NewEncoder(w).Encode(model.TableSchema{
				Name:       "users",
				PrimaryKey: "id",
				Columns: []model.ColumnDef{
					{Name: "id", Type: "INT", PrimaryKey: true},
					{Name: "name", Type: "TEXT"},
				},
			})
			return
		}
		hits++
		_ = json.NewEncoder(w).Encode(model.SQLResponse{Success: true, Result: model.QueryResult{Type: "select"}})
	}))
	defer server.Close()

	instance := New(config, stubNodeLister{nodes: []model.NodeInfo{
		{ID: "g1-n1", HTTPAddr: server.URL, Role: string(shardmeta.RoleShardNode), GroupID: "g1", IsLeader: true},
		{ID: "g2-n1", HTTPAddr: server.URL, Role: string(shardmeta.RoleShardNode), GroupID: "g2", IsLeader: true},
	}}, routeEngine)

	_, err = instance.ExecuteSQL(context.Background(), "SELECT * FROM users WHERE id = 42")
	if err == nil {
		t.Fatalf("ExecuteSQL() error = nil, want migration-in-progress error")
	}
	if !errors.Is(err, ErrShardMigrationBlocked) {
		t.Fatalf("ExecuteSQL() error = %v, want shard migration blocked", err)
	}
	if hits != 0 {
		t.Fatalf("backend hits = %d, want 0 while shard is locked", hits)
	}
}
