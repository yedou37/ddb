package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yedou37/ddb/internal/controller"
	"github.com/yedou37/ddb/internal/coordinator"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/shardmeta"
)

type fakeSQLExecutor struct {
	response model.SQLResponse
	err      error
	lastSQL  string
}

type fakeNodeLister struct {
	nodes []model.NodeInfo
	err   error
}

type fakeMigrator struct {
	migrations []struct {
		shardID   shardmeta.ShardID
		fromGroup shardmeta.GroupID
		toGroup   shardmeta.GroupID
	}
	err   error
	check func(shardmeta.ShardID)
}

func (f *fakeSQLExecutor) ExecuteSQL(_ context.Context, input string) (model.SQLResponse, error) {
	f.lastSQL = input
	return f.response, f.err
}

func (f *fakeNodeLister) ListNodes(context.Context) ([]model.NodeInfo, error) {
	return f.nodes, f.err
}

func (f *fakeMigrator) MigrateShard(_ context.Context, shardID shardmeta.ShardID, sourceGroup, targetGroup shardmeta.GroupID) error {
	if f.err != nil {
		return f.err
	}
	if f.check != nil {
		f.check(shardID)
	}
	f.migrations = append(f.migrations, struct {
		shardID   shardmeta.ShardID
		fromGroup shardmeta.GroupID
		toGroup   shardmeta.GroupID
	}{shardID: shardID, fromGroup: sourceGroup, toGroup: targetGroup})
	return nil
}

func TestNewHandlerHealthConfigAndControlOperations(t *testing.T) {
	service, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	executor := &fakeSQLExecutor{}
	nodeLister := &fakeNodeLister{nodes: []model.NodeInfo{
		{ID: "g1-n1", Role: "shard", GroupID: "g1", HTTPAddr: "http://g1"},
		{ID: "g2-n1", Role: "shard", GroupID: "g2", HTTPAddr: "http://g2"},
	}}
	migrator := &fakeMigrator{check: func(shardID shardmeta.ShardID) {
		if !service.IsShardLocked(shardID) {
			t.Fatalf("service.IsShardLocked(%d) = false, want true during migration", shardID)
		}
	}}
	handler := NewHandler(service, nodeLister, executor, migrator)

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if got, want := health.Code, http.StatusOK; got != want {
		t.Fatalf("/health code = %d, want %d", got, want)
	}

	configRec := httptest.NewRecorder()
	handler.ServeHTTP(configRec, httptest.NewRequest(http.MethodGet, "/config", nil))
	if got, want := configRec.Code, http.StatusOK; got != want {
		t.Fatalf("/config code = %d, want %d", got, want)
	}

	var config shardmeta.ClusterConfig
	if err := json.NewDecoder(configRec.Body).Decode(&config); err != nil {
		t.Fatalf("json.Decode(/config) error = %v", err)
	}
	if got, want := config.TotalShards, shardmeta.DefaultTotalShards; got != want {
		t.Fatalf("config.TotalShards = %d, want %d", got, want)
	}

	shardsRec := httptest.NewRecorder()
	handler.ServeHTTP(shardsRec, httptest.NewRequest(http.MethodGet, "/shards", nil))
	if got, want := shardsRec.Code, http.StatusOK; got != want {
		t.Fatalf("/shards code = %d, want %d", got, want)
	}
	var shards model.ShardsResponse
	if err := json.NewDecoder(shardsRec.Body).Decode(&shards); err != nil {
		t.Fatalf("json.Decode(/shards) error = %v", err)
	}
	if got, want := len(shards.Assignments), shardmeta.DefaultTotalShards; got != want {
		t.Fatalf("len(shards.Assignments) = %d, want %d", got, want)
	}

	groupsRec := httptest.NewRecorder()
	handler.ServeHTTP(groupsRec, httptest.NewRequest(http.MethodGet, "/groups", nil))
	if got, want := groupsRec.Code, http.StatusOK; got != want {
		t.Fatalf("/groups code = %d, want %d", got, want)
	}
	var groups []model.GroupStatus
	if err := json.NewDecoder(groupsRec.Body).Decode(&groups); err != nil {
		t.Fatalf("json.Decode(/groups) error = %v", err)
	}
	if got, want := len(groups), 2; got != want {
		t.Fatalf("len(groups) = %d, want %d", got, want)
	}

	moveBody, marshalErr := json.Marshal(MoveShardRequest{ShardID: 6, GroupID: "g3"})
	if marshalErr != nil {
		t.Fatalf("json.Marshal(move) error = %v", marshalErr)
	}
	moveRec := httptest.NewRecorder()
	handler.ServeHTTP(moveRec, httptest.NewRequest(http.MethodPost, "/move-shard", bytes.NewReader(moveBody)))
	if got, want := moveRec.Code, http.StatusOK; got != want {
		t.Fatalf("/move-shard code = %d, want %d", got, want)
	}

	var movedConfig shardmeta.ClusterConfig
	if err := json.NewDecoder(moveRec.Body).Decode(&movedConfig); err != nil {
		t.Fatalf("json.Decode(/move-shard) error = %v", err)
	}
	groupID, ok := movedConfig.GroupForShard(6)
	if !ok {
		t.Fatalf("GroupForShard(6) ok = false, want true")
	}
	if got, want := groupID, shardmeta.GroupID("g3"); got != want {
		t.Fatalf("GroupForShard(6) = %q, want %q", got, want)
	}
	if got, want := movedConfig.Version, uint64(2); got != want {
		t.Fatalf("movedConfig.Version = %d, want %d", got, want)
	}
	if got, want := len(migrator.migrations), 1; got != want {
		t.Fatalf("len(migrator.migrations) = %d, want %d", got, want)
	}

	rebalanceBody, marshalRebalanceErr := json.Marshal(RebalanceRequest{GroupIDs: []shardmeta.GroupID{"g1", "g2", "g3"}})
	if marshalRebalanceErr != nil {
		t.Fatalf("json.Marshal(rebalance) error = %v", marshalRebalanceErr)
	}
	rebalanceRec := httptest.NewRecorder()
	handler.ServeHTTP(rebalanceRec, httptest.NewRequest(http.MethodPost, "/rebalance", bytes.NewReader(rebalanceBody)))
	if got, want := rebalanceRec.Code, http.StatusOK; got != want {
		t.Fatalf("/rebalance code = %d, want %d", got, want)
	}

	var rebalancedConfig shardmeta.ClusterConfig
	if err := json.NewDecoder(rebalanceRec.Body).Decode(&rebalancedConfig); err != nil {
		t.Fatalf("json.Decode(/rebalance) error = %v", err)
	}
	if got, want := rebalancedConfig.Version, uint64(3); got != want {
		t.Fatalf("rebalancedConfig.Version = %d, want %d", got, want)
	}
	if got, want := rebalancedConfig.Assignments[2].GroupID, shardmeta.GroupID("g3"); got != want {
		t.Fatalf("rebalancedConfig.Assignments[2].GroupID = %q, want %q", got, want)
	}
	if got := len(migrator.migrations); got < 2 {
		t.Fatalf("len(migrator.migrations) = %d, want at least 2 after rebalance", got)
	}
	if service.IsShardLocked(6) {
		t.Fatalf("service.IsShardLocked(6) = true, want false after move-shard completes")
	}
}

func TestNewHandlerControlErrors(t *testing.T) {
	service, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	handler := NewHandler(service, nil, nil, nil)

	methodRec := httptest.NewRecorder()
	handler.ServeHTTP(methodRec, httptest.NewRequest(http.MethodGet, "/move-shard", nil))
	if got, want := methodRec.Code, http.StatusMethodNotAllowed; got != want {
		t.Fatalf("GET /move-shard code = %d, want %d", got, want)
	}

	badMoveBody := bytes.NewBufferString(`{"shard_id":99,"group_id":"g9"}`)
	notFoundRec := httptest.NewRecorder()
	handler.ServeHTTP(notFoundRec, httptest.NewRequest(http.MethodPost, "/move-shard", badMoveBody))
	if got, want := notFoundRec.Code, http.StatusNotFound; got != want {
		t.Fatalf("unknown shard /move-shard code = %d, want %d", got, want)
	}

	badRebalanceBody := bytes.NewBufferString(`{"group_ids":[]}`)
	badRebalanceRec := httptest.NewRecorder()
	handler.ServeHTTP(badRebalanceRec, httptest.NewRequest(http.MethodPost, "/rebalance", badRebalanceBody))
	if got, want := badRebalanceRec.Code, http.StatusBadRequest; got != want {
		t.Fatalf("bad /rebalance code = %d, want %d", got, want)
	}
}

func TestNewHandlerSQL(t *testing.T) {
	service, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	executor := &fakeSQLExecutor{
		response: model.SQLResponse{
			Success: true,
			Result:  model.QueryResult{Type: "select"},
		},
	}
	handler := NewHandler(service, nil, executor, nil)

	body := bytes.NewBufferString(`{"sql":"SELECT * FROM users WHERE id = 1"}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sql", body))
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("/sql code = %d, want %d", got, want)
	}
	if got, want := executor.lastSQL, "SELECT * FROM users WHERE id = 1"; got != want {
		t.Fatalf("executor.lastSQL = %q, want %q", got, want)
	}
}

func TestNewHandlerSQLReturnsRetryDuringShardMigration(t *testing.T) {
	service, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	executor := &fakeSQLExecutor{
		err: coordinator.ShardMigrationError{ShardID: 3},
	}
	handler := NewHandler(service, nil, executor, nil)

	body := bytes.NewBufferString(`{"sql":"SELECT * FROM users WHERE id = 1"}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sql", body))
	if got, want := rec.Code, http.StatusServiceUnavailable; got != want {
		t.Fatalf("/sql code = %d, want %d", got, want)
	}
	if got, want := rec.Header().Get("Retry-After"), "1"; got != want {
		t.Fatalf("Retry-After = %q, want %q", got, want)
	}
}

func TestNewHandlerDashboardRoutes(t *testing.T) {
	service, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	shardStatusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/status":
			writeJSON(w, http.StatusOK, model.StatusResponse{
				NodeID:   "g1-n1",
				HTTPAddr: "http://g1",
				Role:     "shard",
				Leader:   "g1-n1",
				Tables:   []string{"users"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer shardStatusServer.Close()

	controlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer controlServer.Close()

	nodeLister := &fakeNodeLister{nodes: []model.NodeInfo{
		{ID: "ctrl-1", Role: "controller", HTTPAddr: controlServer.URL},
		{ID: "api-1", Role: "apiserver", HTTPAddr: controlServer.URL},
		{ID: "g1-n1", Role: "shard", GroupID: "g1", HTTPAddr: shardStatusServer.URL, IsLeader: true},
	}}

	handler := NewHandler(service, nodeLister, nil, nil)

	overviewRec := httptest.NewRecorder()
	handler.ServeHTTP(overviewRec, httptest.NewRequest(http.MethodGet, "/dashboard/api/overview", nil))
	if got, want := overviewRec.Code, http.StatusOK; got != want {
		t.Fatalf("/dashboard/api/overview code = %d, want %d", got, want)
	}

	var overview dashboardOverview
	if err := json.NewDecoder(overviewRec.Body).Decode(&overview); err != nil {
		t.Fatalf("json.Decode(/dashboard/api/overview) error = %v", err)
	}
	if got, want := overview.Summary.TotalNodes, 3; got != want {
		t.Fatalf("overview.Summary.TotalNodes = %d, want %d", got, want)
	}
	if got, want := overview.Summary.ReachableNodes, 3; got != want {
		t.Fatalf("overview.Summary.ReachableNodes = %d, want %d", got, want)
	}
	if got, want := overview.Nodes[2].TableCount, 1; got != want {
		t.Fatalf("overview.Nodes[2].TableCount = %d, want %d", got, want)
	}

	pageRec := httptest.NewRecorder()
	handler.ServeHTTP(pageRec, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))
	if got, want := pageRec.Code, http.StatusOK; got != want {
		t.Fatalf("/dashboard/ code = %d, want %d", got, want)
	}
	if body := pageRec.Body.String(); !strings.Contains(body, "Cluster Dashboard") {
		t.Fatalf("/dashboard/ body missing dashboard title")
	}
}

func TestNewHandlerDashboardTableData(t *testing.T) {
	service, err := controller.NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	executor := &fakeSQLExecutor{
		response: model.SQLResponse{
			Success: true,
			Result: model.QueryResult{
				Type:    "select",
				Columns: []string{"id", "name"},
				Rows:    [][]any{{float64(1), "alice"}, {float64(2), "bob"}},
			},
		},
	}
	handler := NewHandler(service, nil, executor, nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard/api/table-data?table=users", nil))
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("/dashboard/api/table-data code = %d, want %d", got, want)
	}
	if got, want := executor.lastSQL, "SELECT * FROM users"; got != want {
		t.Fatalf("executor.lastSQL = %q, want %q", got, want)
	}

	var payload dashboardTableData
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("json.Decode(/dashboard/api/table-data) error = %v", err)
	}
	if got, want := payload.Table, "users"; got != want {
		t.Fatalf("payload.Table = %q, want %q", got, want)
	}
	if got, want := len(payload.Result.Rows), 2; got != want {
		t.Fatalf("len(payload.Result.Rows) = %d, want %d", got, want)
	}
}
