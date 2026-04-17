package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/yedou37/ddb/internal/apiserver"
	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/controller"
	"github.com/yedou37/ddb/internal/coordinator"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/router"
	"github.com/yedou37/ddb/internal/shardmeta"
)

type staticNodeLister struct {
	nodes []model.NodeInfo
}

func (s staticNodeLister) ListNodes(context.Context) ([]model.NodeInfo, error) {
	return s.nodes, nil
}

func TestShardMoveMigratesRowsAndUpdatesControlPlane(t *testing.T) {
	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	_ = startNode(t, config.ServerConfig{
		NodeID:    "g1-n1",
		Role:      shardmeta.RoleShardNode,
		GroupID:   "g1",
		HTTPAddr:  http1,
		RaftAddr:  raft1,
		RaftDir:   filepath.Join(baseDir, "raft-g1"),
		DBPath:    filepath.Join(baseDir, "db-g1.db"),
		Bootstrap: true,
	})
	_ = startNode(t, config.ServerConfig{
		NodeID:    "g2-n1",
		Role:      shardmeta.RoleShardNode,
		GroupID:   "g2",
		HTTPAddr:  http2,
		RaftAddr:  raft2,
		RaftDir:   filepath.Join(baseDir, "raft-g2"),
		DBPath:    filepath.Join(baseDir, "db-g2.db"),
		Bootstrap: true,
	})
	_ = startNode(t, config.ServerConfig{
		NodeID:    "g3-n1",
		Role:      shardmeta.RoleShardNode,
		GroupID:   "g3",
		HTTPAddr:  http3,
		RaftAddr:  raft3,
		RaftDir:   filepath.Join(baseDir, "raft-g3"),
		DBPath:    filepath.Join(baseDir, "db-g3.db"),
		Bootstrap: true,
	})
	waitForHealth(t, http1)
	waitForHealth(t, http2)
	waitForHealth(t, http3)
	waitForLeader(t, http1)
	waitForLeader(t, http2)
	waitForLeader(t, http3)

	controlService, err := controller.NewBootstrapService(
		shardmeta.DefaultTotalShards,
		[]shardmeta.GroupID{"g1", "g2"},
		controller.NewMemoryStore(),
	)
	if err != nil {
		t.Fatalf("NewBootstrapService() error = %v", err)
	}
	routeEngine, err := router.New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("router.New() error = %v", err)
	}
	nodeLister := staticNodeLister{nodes: []model.NodeInfo{
		{ID: "g1-n1", HTTPAddr: http1, Role: string(shardmeta.RoleShardNode), GroupID: "g1", IsLeader: true},
		{ID: "g2-n1", HTTPAddr: http2, Role: string(shardmeta.RoleShardNode), GroupID: "g2", IsLeader: true},
		{ID: "g3-n1", HTTPAddr: http3, Role: string(shardmeta.RoleShardNode), GroupID: "g3", IsLeader: true},
	}}
	coord := coordinator.New(controlService, nodeLister, routeEngine)
	controlServer := httptest.NewServer(apiserver.NewHandler(controlService, nodeLister, coord, coord))
	defer controlServer.Close()

	controlExecSQL(t, controlServer.URL, "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)")

	key, shardID, fromGroup := findKeyForGroup(t, routeEngine, controlService.CurrentConfig(), "users", "g1")
	controlExecSQL(t, controlServer.URL, buildInsertStatement("users", []any{key, "alice"}))
	waitForNamedRowCount(t, http1, "users", 1)

	moveBody, err := json.Marshal(apiserver.MoveShardRequest{ShardID: shardID, GroupID: "g3"})
	if err != nil {
		t.Fatalf("json.Marshal(move-shard) error = %v", err)
	}
	resp, err := http.Post(controlServer.URL+"/move-shard", "application/json", bytes.NewReader(moveBody))
	if err != nil {
		t.Fatalf("http.Post(/move-shard) error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/move-shard status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	shards := controlGetJSON[model.ShardsResponse](t, controlServer.URL+"/shards")
	found := false
	for _, assignment := range shards.Assignments {
		if assignment.ShardID == uint32(shardID) {
			found = true
			if got, want := assignment.GroupID, "g3"; got != want {
				t.Fatalf("moved shard group = %q, want %q", got, want)
			}
		}
	}
	if !found {
		t.Fatalf("moved shard %d not found in /shards", shardID)
	}

	switch fromGroup {
	case "g1":
		waitForNamedRowCount(t, http1, "users", 0)
	case "g2":
		waitForNamedRowCount(t, http2, "users", 0)
	}
	waitForNamedRowCount(t, http3, "users", 1)

	result := controlExecSQL(t, controlServer.URL, fmt.Sprintf("SELECT * FROM users WHERE id = %d", key))
	if got, want := len(result.Result.Rows), 1; got != want {
		t.Fatalf("len(result.Result.Rows) = %d, want %d", got, want)
	}

	groups := controlGetJSON[[]model.GroupStatus](t, controlServer.URL+"/groups")
	group3Found := false
	for _, group := range groups {
		if group.GroupID == "g3" {
			group3Found = true
			if len(group.Shards) == 0 {
				t.Fatalf("group3.Shards = empty, want migrated shard")
			}
		}
	}
	if !group3Found {
		t.Fatalf("group3 not found in /groups")
	}
}

func findKeyForGroup(t *testing.T, routeEngine *router.Router, config shardmeta.ClusterConfig, table, group string) (int, shardmeta.ShardID, string) {
	t.Helper()
	for key := 1; key < 128; key++ {
		result, err := routeEngine.Route(table, key, config)
		if err != nil {
			t.Fatalf("routeEngine.Route(%d) error = %v", key, err)
		}
		if string(result.GroupID) == group {
			return key, result.ShardID, string(result.GroupID)
		}
	}
	t.Fatalf("no key found for group %s", group)
	return 0, 0, ""
}

func buildInsertStatement(table string, values []any) string {
	return fmt.Sprintf("INSERT INTO %s VALUES (%v, '%v')", table, values[0], values[1])
}

func controlExecSQL(t *testing.T, baseURL, statement string) model.SQLResponse {
	t.Helper()
	body, err := json.Marshal(model.SQLRequest{SQL: statement})
	if err != nil {
		t.Fatalf("json.Marshal(SQLRequest) error = %v", err)
	}
	resp, err := http.Post(baseURL+"/sql", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http.Post(/sql) error = %v", err)
	}
	defer resp.Body.Close()

	var parsed model.SQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		t.Fatalf("json.Decode(/sql) error = %v", err)
	}
	if resp.StatusCode != http.StatusOK || !parsed.Success {
		t.Fatalf("SQL %q failed: status=%d body=%+v", statement, resp.StatusCode, parsed)
	}
	return parsed
}

func controlGetJSON[T any](t *testing.T, url string) T {
	t.Helper()
	var result T
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("http.Get(%s) error = %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("http.Get(%s) status = %d, want %d", url, resp.StatusCode, http.StatusOK)
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("json.Decode(%s) error = %v", url, err)
	}
	return result
}
