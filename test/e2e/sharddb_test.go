package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

type blockingMigrator struct {
	inner   apiserver.ShardMigrator
	started chan struct{}
	release chan struct{}
}

func (m *blockingMigrator) MigrateShard(ctx context.Context, shardID shardmeta.ShardID, sourceGroup, targetGroup shardmeta.GroupID) error {
	select {
	case <-m.started:
	default:
		close(m.started)
	}
	<-m.release
	return m.inner.MigrateShard(ctx, shardID, sourceGroup, targetGroup)
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

func TestScatterSelectAndEqualityJoinAcrossShards(t *testing.T) {
	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)

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
	waitForHealth(t, http1)
	waitForHealth(t, http2)
	waitForLeader(t, http1)
	waitForLeader(t, http2)

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
	}}
	coord := coordinator.New(controlService, nodeLister, routeEngine)
	controlServer := httptest.NewServer(apiserver.NewHandler(controlService, nodeLister, coord, coord))
	defer controlServer.Close()

	controlExecSQL(t, controlServer.URL, "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)")
	controlExecSQL(t, controlServer.URL, "CREATE TABLE orders (id INT PRIMARY KEY, user_id INT, item TEXT)")

	keyG1, _, _ := findKeyForGroup(t, routeEngine, controlService.CurrentConfig(), "users", "g1")
	keyG2, _, _ := findKeyForGroup(t, routeEngine, controlService.CurrentConfig(), "users", "g2")
	controlExecSQL(t, controlServer.URL, buildInsertStatement("users", []any{keyG1, "alice"}))
	controlExecSQL(t, controlServer.URL, buildInsertStatement("users", []any{keyG2, "bob"}))
	controlExecSQL(t, controlServer.URL, buildInsertStatement("orders", []any{100, keyG1, "book"}))
	controlExecSQL(t, controlServer.URL, buildInsertStatement("orders", []any{200, keyG2, "pen"}))

	selectResult := controlExecSQL(t, controlServer.URL, "SELECT * FROM users")
	if got, want := len(selectResult.Result.Rows), 2; got != want {
		t.Fatalf("len(selectResult.Result.Rows) = %d, want %d", got, want)
	}

	joinResult := controlExecSQL(t, controlServer.URL, "SELECT * FROM users JOIN orders ON users.id = orders.user_id")
	if got, want := joinResult.Result.Type, "join"; got != want {
		t.Fatalf("joinResult.Result.Type = %q, want %q", got, want)
	}
	if got, want := len(joinResult.Result.Rows), 2; got != want {
		t.Fatalf("len(joinResult.Result.Rows) = %d, want %d", got, want)
	}
	if got, want := joinResult.Result.Columns[0], "users.id"; got != want {
		t.Fatalf("joinResult.Result.Columns[0] = %q, want %q", got, want)
	}

	status, response, _ := controlExecSQLWithStatus(t, controlServer.URL, "SELECT * FROM users JOIN orders ON users.id > orders.user_id")
	if got, want := status, http.StatusBadRequest; got != want {
		t.Fatalf("non-equality JOIN status = %d, want %d", got, want)
	}
	if !strings.Contains(response.Error, "only equality JOIN is supported") {
		t.Fatalf("non-equality JOIN error = %q, want equality-join hint", response.Error)
	}
}

func TestShardMoveReturnsRetryDuringMigration(t *testing.T) {
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
	migrator := &blockingMigrator{
		inner:   coord,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	controlServer := httptest.NewServer(apiserver.NewHandler(controlService, nodeLister, coord, migrator))
	defer controlServer.Close()

	controlExecSQL(t, controlServer.URL, "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)")
	key, shardID, _ := findKeyForGroup(t, routeEngine, controlService.CurrentConfig(), "users", "g1")
	otherKey := findAnotherKeyForShard(t, routeEngine, controlService.CurrentConfig(), "users", shardID, key)
	controlExecSQL(t, controlServer.URL, buildInsertStatement("users", []any{key, "alice"}))
	waitForNamedRowCount(t, http1, "users", 1)

	moveBody, err := json.Marshal(apiserver.MoveShardRequest{ShardID: shardID, GroupID: "g3"})
	if err != nil {
		t.Fatalf("json.Marshal(move-shard) error = %v", err)
	}

	moveErrCh := make(chan error, 1)
	go func() {
		resp, postErr := http.Post(controlServer.URL+"/move-shard", "application/json", bytes.NewReader(moveBody))
		if postErr != nil {
			moveErrCh <- postErr
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			moveErrCh <- fmt.Errorf("/move-shard status = %d, want %d", resp.StatusCode, http.StatusOK)
			return
		}
		moveErrCh <- nil
	}()

	<-migrator.started

	status, response, headers := controlExecSQLWithStatus(t, controlServer.URL, fmt.Sprintf("SELECT * FROM users WHERE id = %d", key))
	if got, want := status, http.StatusServiceUnavailable; got != want {
		t.Fatalf("SELECT during migration status = %d, want %d", got, want)
	}
	if got, want := headers.Get("Retry-After"), "1"; got != want {
		t.Fatalf("Retry-After = %q, want %q", got, want)
	}
	if response.Success {
		t.Fatalf("response.Success = true, want false during shard migration")
	}
	if !strings.Contains(response.Error, "retry later") {
		t.Fatalf("response.Error = %q, want retry-later hint", response.Error)
	}

	status, response, headers = controlExecSQLWithStatus(t, controlServer.URL, buildInsertStatement("users", []any{otherKey, "bob"}))
	if got, want := status, http.StatusServiceUnavailable; got != want {
		t.Fatalf("INSERT during migration status = %d, want %d", got, want)
	}
	if got, want := headers.Get("Retry-After"), "1"; got != want {
		t.Fatalf("INSERT Retry-After = %q, want %q", got, want)
	}
	if response.Success {
		t.Fatalf("INSERT response.Success = true, want false during shard migration")
	}
	if !strings.Contains(response.Error, "retry later") {
		t.Fatalf("INSERT response.Error = %q, want retry-later hint", response.Error)
	}

	status, response, headers = controlExecSQLWithStatus(t, controlServer.URL, fmt.Sprintf("DELETE FROM users WHERE id = %d", key))
	if got, want := status, http.StatusServiceUnavailable; got != want {
		t.Fatalf("DELETE during migration status = %d, want %d", got, want)
	}
	if got, want := headers.Get("Retry-After"), "1"; got != want {
		t.Fatalf("DELETE Retry-After = %q, want %q", got, want)
	}
	if response.Success {
		t.Fatalf("DELETE response.Success = true, want false during shard migration")
	}
	if !strings.Contains(response.Error, "retry later") {
		t.Fatalf("DELETE response.Error = %q, want retry-later hint", response.Error)
	}

	close(migrator.release)
	if moveErr := <-moveErrCh; moveErr != nil {
		t.Fatalf("move-shard failed: %v", moveErr)
	}

	result := controlExecSQL(t, controlServer.URL, fmt.Sprintf("SELECT * FROM users WHERE id = %d", key))
	if got, want := len(result.Result.Rows), 1; got != want {
		t.Fatalf("len(result.Result.Rows) = %d, want %d after move-shard completes", got, want)
	}
	missing := controlExecSQL(t, controlServer.URL, fmt.Sprintf("SELECT * FROM users WHERE id = %d", otherKey))
	if got, want := len(missing.Result.Rows), 0; got != want {
		t.Fatalf("len(missing.Result.Rows) = %d, want %d for rejected insert", got, want)
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

func findAnotherKeyForShard(t *testing.T, routeEngine *router.Router, config shardmeta.ClusterConfig, table string, shardID shardmeta.ShardID, exclude int) int {
	t.Helper()
	for key := 1; key < 256; key++ {
		if key == exclude {
			continue
		}
		result, err := routeEngine.Route(table, key, config)
		if err != nil {
			t.Fatalf("routeEngine.Route(%d) error = %v", key, err)
		}
		if result.ShardID == shardID {
			return key
		}
	}
	t.Fatalf("no alternate key found for shard %d", shardID)
	return 0
}

func buildInsertStatement(table string, values []any) string {
	literals := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			literals = append(literals, fmt.Sprintf("'%s'", typed))
		default:
			literals = append(literals, fmt.Sprintf("%v", typed))
		}
	}
	return fmt.Sprintf("INSERT INTO %s VALUES (%s)", table, strings.Join(literals, ", "))
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

func controlExecSQLWithStatus(t *testing.T, baseURL, statement string) (int, model.SQLResponse, http.Header) {
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
	return resp.StatusCode, parsed, resp.Header.Clone()
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
