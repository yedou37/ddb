package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/router"
	"github.com/yedou37/ddb/internal/shardmeta"
)

var (
	buildServerBinaryOnce sync.Once
	buildServerBinaryPath string
	buildServerBinaryErr  error
)

func TestProcessClusterReplicatesWritesAcrossRealServerProcesses(t *testing.T) {
	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	node1 := startServerProcess(t, config.ServerConfig{
		NodeID:    "node1",
		HTTPAddr:  http1,
		RaftAddr:  raft1,
		RaftDir:   filepath.Join(baseDir, "raft1"),
		DBPath:    filepath.Join(baseDir, "db1.db"),
		Bootstrap: true,
	})
	waitForHealth(t, http1)
	waitForLeader(t, http1)

	_ = startServerProcess(t, config.ServerConfig{
		NodeID:   "node2",
		HTTPAddr: http2,
		RaftAddr: raft2,
		RaftDir:  filepath.Join(baseDir, "raft2"),
		DBPath:   filepath.Join(baseDir, "db2.db"),
		JoinAddr: http1,
	})
	waitForHealthWithin(t, http2, 30*time.Second)

	_ = startServerProcess(t, config.ServerConfig{
		NodeID:   "node3",
		HTTPAddr: http3,
		RaftAddr: raft3,
		RaftDir:  filepath.Join(baseDir, "raft3"),
		DBPath:   filepath.Join(baseDir, "db3.db"),
		JoinAddr: http1,
	})
	waitForHealthWithin(t, http3, 30*time.Second)
	waitForMemberCountWithin(t, http1, 3, 20*time.Second)

	execSQL(t, http1, "CREATE TABLE process_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO process_books VALUES (1, 'raft')")
	execSQL(t, http1, "INSERT INTO process_books VALUES (2, 'cluster')")

	waitForNamedRowCountWithin(t, http2, "process_books", 2, 20*time.Second)
	waitForNamedRowCountWithin(t, http3, "process_books", 2, 20*time.Second)

	node1.stop(t)
	waitForWriteSuccess(t, []string{http2, http3}, "INSERT INTO process_books VALUES (3, 'after-failover')")
	waitForNamedRowCountWithin(t, http2, "process_books", 3, 20*time.Second)
	waitForNamedRowCountWithin(t, http3, "process_books", 3, 20*time.Second)
}

func TestProcessRemovedNodeRequiresRejoinFlag(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "node1",
		HTTPAddr:      http1,
		RaftAddr:      raft1,
		RaftDir:       filepath.Join(baseDir, "raft1"),
		DBPath:        filepath.Join(baseDir, "db1.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealth(t, http1)
	waitForLeader(t, http1)

	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "node2",
		HTTPAddr:      http2,
		RaftAddr:      raft2,
		RaftDir:       filepath.Join(baseDir, "raft2"),
		DBPath:        filepath.Join(baseDir, "db2.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealthWithin(t, http2, 30*time.Second)

	node3Cfg := config.ServerConfig{
		NodeID:        "node3",
		HTTPAddr:      http3,
		RaftAddr:      raft3,
		RaftDir:       filepath.Join(baseDir, "raft3"),
		DBPath:        filepath.Join(baseDir, "db3.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node3 := startServerProcess(t, node3Cfg)
	waitForHealthWithin(t, http3, 30*time.Second)
	waitForMemberCountWithin(t, http1, 3, 20*time.Second)

	postJSON(t, "http://"+http1+"/remove", map[string]string{"node_id": "node3"})
	waitForMemberStatusWithin(t, http1, "node3", func(member model.ClusterMember) bool {
		return !member.InRaft && member.Removed && member.Status == "removed"
	}, 20*time.Second)

	node3.stop(t)

	restart := startDetachedServerProcess(t, node3Cfg)
	exitCode, logs := waitForProcessExitWithin(t, restart, 10*time.Second)
	if exitCode == 0 {
		t.Fatalf("removed node restarted without --rejoin unexpectedly succeeded; logs:\n%s", logs)
	}
}

func TestProcessAutoDiscoveryJoinAfterLeaderFailover(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)
	http4, raft4 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	node1 := startServerProcess(t, config.ServerConfig{
		NodeID:        "node1",
		HTTPAddr:      http1,
		RaftAddr:      raft1,
		RaftDir:       filepath.Join(baseDir, "raft1"),
		DBPath:        filepath.Join(baseDir, "db1.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealth(t, http1)
	waitForLeader(t, http1)

	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "node2",
		HTTPAddr:      http2,
		RaftAddr:      raft2,
		RaftDir:       filepath.Join(baseDir, "raft2"),
		DBPath:        filepath.Join(baseDir, "db2.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealthWithin(t, http2, 30*time.Second)

	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "node3",
		HTTPAddr:      http3,
		RaftAddr:      raft3,
		RaftDir:       filepath.Join(baseDir, "raft3"),
		DBPath:        filepath.Join(baseDir, "db3.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealthWithin(t, http3, 30*time.Second)
	waitForMemberCountWithin(t, http1, 3, 20*time.Second)

	execSQL(t, http1, "CREATE TABLE process_discovery_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO process_discovery_books VALUES (1, 'before-failover')")
	waitForNamedRowCountWithin(t, http2, "process_discovery_books", 1, 20*time.Second)
	waitForNamedRowCountWithin(t, http3, "process_discovery_books", 1, 20*time.Second)

	node1.stop(t)
	waitForWriteSuccess(t, []string{http2, http3}, "INSERT INTO process_discovery_books VALUES (2, 'after-failover')")
	waitForMemberStatusOnAnyWithin(t, []string{http2, http3}, "node1", func(member model.ClusterMember) bool {
		return !member.Online && member.Status == "offline-voter"
	}, 25*time.Second)
	waitForLeaderMemberOnAnyWithin(t, []string{http2, http3}, []string{"node2", "node3"}, 25*time.Second)

	node4 := startDetachedServerProcess(t, config.ServerConfig{
		NodeID:        "node4",
		HTTPAddr:      http4,
		RaftAddr:      raft4,
		RaftDir:       filepath.Join(baseDir, "raft4"),
		DBPath:        filepath.Join(baseDir, "db4.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	t.Cleanup(func() {
		node4.stop(t)
	})
	waitForProcessHealthWithin(t, node4, http4, 30*time.Second)
	waitForMemberCountOnAnyWithin(t, []string{http2, http3, http4}, 4, 25*time.Second)
	waitForNamedRowCountWithin(t, http4, "process_discovery_books", 2, 25*time.Second)

	waitForWriteSuccess(t, []string{http2, http3, http4}, "INSERT INTO process_discovery_books VALUES (3, 'after-node4-join')")
	waitForNamedRowCountWithin(t, http4, "process_discovery_books", 3, 25*time.Second)
}

func TestProcessRemovedNodeRejoinsAndCatchesUp(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "node1",
		HTTPAddr:      http1,
		RaftAddr:      raft1,
		RaftDir:       filepath.Join(baseDir, "raft1"),
		DBPath:        filepath.Join(baseDir, "db1.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealth(t, http1)
	waitForLeader(t, http1)

	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "node2",
		HTTPAddr:      http2,
		RaftAddr:      raft2,
		RaftDir:       filepath.Join(baseDir, "raft2"),
		DBPath:        filepath.Join(baseDir, "db2.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealthWithin(t, http2, 30*time.Second)

	node3Cfg := config.ServerConfig{
		NodeID:        "node3",
		HTTPAddr:      http3,
		RaftAddr:      raft3,
		RaftDir:       filepath.Join(baseDir, "raft3"),
		DBPath:        filepath.Join(baseDir, "db3.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node3 := startServerProcess(t, node3Cfg)
	waitForHealthWithin(t, http3, 30*time.Second)
	waitForMemberCountWithin(t, http1, 3, 20*time.Second)

	execSQL(t, http1, "CREATE TABLE process_rejoin_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO process_rejoin_books VALUES (1, 'before-remove')")
	waitForNamedRowCountWithin(t, http3, "process_rejoin_books", 1, 20*time.Second)

	postJSON(t, "http://"+http1+"/remove", model.RemoveRequest{NodeID: "node3"})
	waitForMemberStatusWithin(t, http1, "node3", func(member model.ClusterMember) bool {
		return !member.InRaft && member.Removed && member.Status == "removed"
	}, 20*time.Second)
	node3.stop(t)

	waitForWriteSuccess(t, []string{http1, http2}, "INSERT INTO process_rejoin_books VALUES (2, 'while-removed')")
	waitForNamedRowCountWithin(t, http2, "process_rejoin_books", 2, 20*time.Second)

	rejoinCfg := node3Cfg
	rejoinCfg.Rejoin = true
	_ = startServerProcess(t, rejoinCfg)
	waitForHealthWithin(t, http3, 30*time.Second)
	waitForMemberStatusOnAnyWithin(t, []string{http1, http2, http3}, "node3", func(member model.ClusterMember) bool {
		return member.InRaft && member.Online && !member.Removed && member.Status == "online-voter"
	}, 25*time.Second)
	waitForNamedRowCountWithin(t, http3, "process_rejoin_books", 2, 25*time.Second)

	waitForWriteSuccess(t, []string{http1, http2, http3}, "INSERT INTO process_rejoin_books VALUES (3, 'after-rejoin')")
	waitForNamedRowCountWithin(t, http3, "process_rejoin_books", 3, 25*time.Second)
}

func TestProcessShardDBMoveShardUpdatesSharedControlPlane(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	shard1HTTP, shard1Raft := reserveAddr(t), reserveAddr(t)
	shard2HTTP, shard2Raft := reserveAddr(t), reserveAddr(t)
	shard3HTTP, shard3Raft := reserveAddr(t), reserveAddr(t)
	controllerHTTP, controllerRaft := reserveAddr(t), reserveAddr(t)
	apiHTTP, apiRaft := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "g1-n1",
		Role:          shardmeta.RoleShardNode,
		GroupID:       "g1",
		HTTPAddr:      shard1HTTP,
		RaftAddr:      shard1Raft,
		RaftDir:       filepath.Join(baseDir, "raft-g1"),
		DBPath:        filepath.Join(baseDir, "db-g1.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "g2-n1",
		Role:          shardmeta.RoleShardNode,
		GroupID:       "g2",
		HTTPAddr:      shard2HTTP,
		RaftAddr:      shard2Raft,
		RaftDir:       filepath.Join(baseDir, "raft-g2"),
		DBPath:        filepath.Join(baseDir, "db-g2.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "g3-n1",
		Role:          shardmeta.RoleShardNode,
		GroupID:       "g3",
		HTTPAddr:      shard3HTTP,
		RaftAddr:      shard3Raft,
		RaftDir:       filepath.Join(baseDir, "raft-g3"),
		DBPath:        filepath.Join(baseDir, "db-g3.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealthWithin(t, shard1HTTP, 30*time.Second)
	waitForHealthWithin(t, shard2HTTP, 30*time.Second)
	waitForHealthWithin(t, shard3HTTP, 30*time.Second)
	waitForLeader(t, shard1HTTP)
	waitForLeader(t, shard2HTTP)
	waitForLeader(t, shard3HTTP)

	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "ctrl-1",
		Role:          shardmeta.RoleController,
		HTTPAddr:      controllerHTTP,
		RaftAddr:      controllerRaft,
		RaftDir:       filepath.Join(baseDir, "raft-controller"),
		DBPath:        filepath.Join(baseDir, "controller.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "api-1",
		Role:          shardmeta.RoleAPIServer,
		HTTPAddr:      apiHTTP,
		RaftAddr:      apiRaft,
		RaftDir:       filepath.Join(baseDir, "raft-apiserver"),
		DBPath:        filepath.Join(baseDir, "apiserver.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealthWithin(t, controllerHTTP, 30*time.Second)
	waitForHealthWithin(t, apiHTTP, 30*time.Second)

	controllerURL := "http://" + controllerHTTP
	apiURL := "http://" + apiHTTP
	routeEngine, err := router.New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("router.New() error = %v", err)
	}

	controllerConfig := controlGetJSON[shardmeta.ClusterConfig](t, controllerURL+"/config")
	apiConfig := controlGetJSON[shardmeta.ClusterConfig](t, apiURL+"/config")
	if got, want := apiConfig.Version, controllerConfig.Version; got != want {
		t.Fatalf("api config version = %d, want %d", got, want)
	}

	controlExecSQL(t, apiURL, "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)")
	key, shardID, fromGroup := findKeyForGroup(t, routeEngine, apiConfig, "users", "g1")
	controlExecSQL(t, apiURL, buildInsertStatement("users", []any{key, "alice"}))
	waitForNamedRowCountWithin(t, shard1HTTP, "users", 1, 20*time.Second)

	moveShard(t, controllerURL, shardID, "g3")
	waitForShardAssignmentWithin(t, apiURL, shardID, "g3", 25*time.Second)
	waitForNamedRowCountWithin(t, shard3HTTP, "users", 1, 25*time.Second)

	switch fromGroup {
	case "g1":
		waitForNamedRowCountWithin(t, shard1HTTP, "users", 0, 25*time.Second)
	case "g2":
		waitForNamedRowCountWithin(t, shard2HTTP, "users", 0, 25*time.Second)
	}

	result := controlExecSQL(t, apiURL, buildSelectByID("users", key))
	if got, want := len(result.Result.Rows), 1; got != want {
		t.Fatalf("len(result.Result.Rows) = %d, want %d", got, want)
	}

	groups := controlGetJSON[[]model.GroupStatus](t, apiURL+"/groups")
	if !groupContainsShard(groups, "g3", uint32(shardID)) {
		t.Fatalf("group g3 did not report moved shard %d", shardID)
	}
}

func TestProcessShardDBAPIServerRestartReloadsConfigFromEtcd(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	shard1HTTP, shard1Raft := reserveAddr(t), reserveAddr(t)
	shard2HTTP, shard2Raft := reserveAddr(t), reserveAddr(t)
	shard3HTTP, shard3Raft := reserveAddr(t), reserveAddr(t)
	controllerHTTP, controllerRaft := reserveAddr(t), reserveAddr(t)
	apiHTTP, apiRaft := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "g1-n1",
		Role:          shardmeta.RoleShardNode,
		GroupID:       "g1",
		HTTPAddr:      shard1HTTP,
		RaftAddr:      shard1Raft,
		RaftDir:       filepath.Join(baseDir, "raft-g1"),
		DBPath:        filepath.Join(baseDir, "db-g1.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "g2-n1",
		Role:          shardmeta.RoleShardNode,
		GroupID:       "g2",
		HTTPAddr:      shard2HTTP,
		RaftAddr:      shard2Raft,
		RaftDir:       filepath.Join(baseDir, "raft-g2"),
		DBPath:        filepath.Join(baseDir, "db-g2.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "g3-n1",
		Role:          shardmeta.RoleShardNode,
		GroupID:       "g3",
		HTTPAddr:      shard3HTTP,
		RaftAddr:      shard3Raft,
		RaftDir:       filepath.Join(baseDir, "raft-g3"),
		DBPath:        filepath.Join(baseDir, "db-g3.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealthWithin(t, shard1HTTP, 30*time.Second)
	waitForHealthWithin(t, shard2HTTP, 30*time.Second)
	waitForHealthWithin(t, shard3HTTP, 30*time.Second)
	waitForLeader(t, shard1HTTP)
	waitForLeader(t, shard2HTTP)
	waitForLeader(t, shard3HTTP)

	_ = startServerProcess(t, config.ServerConfig{
		NodeID:        "ctrl-1",
		Role:          shardmeta.RoleController,
		HTTPAddr:      controllerHTTP,
		RaftAddr:      controllerRaft,
		RaftDir:       filepath.Join(baseDir, "raft-controller"),
		DBPath:        filepath.Join(baseDir, "controller.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	})
	waitForHealthWithin(t, controllerHTTP, 30*time.Second)

	apiCfg := config.ServerConfig{
		NodeID:        "api-1",
		Role:          shardmeta.RoleAPIServer,
		HTTPAddr:      apiHTTP,
		RaftAddr:      apiRaft,
		RaftDir:       filepath.Join(baseDir, "raft-apiserver"),
		DBPath:        filepath.Join(baseDir, "apiserver.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	apiNode := startServerProcess(t, apiCfg)
	waitForHealthWithin(t, apiHTTP, 30*time.Second)

	controllerURL := "http://" + controllerHTTP
	apiURL := "http://" + apiHTTP
	routeEngine, err := router.New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("router.New() error = %v", err)
	}

	controlExecSQL(t, apiURL, "CREATE TABLE books (id INT PRIMARY KEY, name TEXT)")
	configBefore := controlGetJSON[shardmeta.ClusterConfig](t, controllerURL+"/config")
	key, shardID, _ := findKeyForGroup(t, routeEngine, configBefore, "books", "g1")
	controlExecSQL(t, apiURL, buildInsertStatement("books", []any{key, "persisted"}))
	waitForNamedRowCountWithin(t, shard1HTTP, "books", 1, 20*time.Second)

	moveShard(t, controllerURL, shardID, "g3")
	waitForShardAssignmentWithin(t, apiURL, shardID, "g3", 25*time.Second)
	waitForNamedRowCountWithin(t, shard3HTTP, "books", 1, 25*time.Second)

	apiNode.stop(t)

	restarted := startDetachedServerProcess(t, apiCfg)
	t.Cleanup(func() {
		restarted.stop(t)
	})
	waitForProcessHealthWithin(t, restarted, apiHTTP, 30*time.Second)
	waitForShardAssignmentWithin(t, apiURL, shardID, "g3", 25*time.Second)

	result := controlExecSQL(t, apiURL, buildSelectByID("books", key))
	if got, want := len(result.Result.Rows), 1; got != want {
		t.Fatalf("len(result.Result.Rows) after apiserver restart = %d, want %d", got, want)
	}
	waitForNamedRowCountWithin(t, shard3HTTP, "books", 1, 20*time.Second)
}

type processNode struct {
	cmd  *exec.Cmd
	logs *lockedBuffer
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func startServerProcess(t *testing.T, cfg config.ServerConfig) *processNode {
	t.Helper()

	node := startDetachedServerProcess(t, cfg)
	t.Cleanup(func() {
		node.stop(t)
	})
	return node
}

func startDetachedServerProcess(t *testing.T, cfg config.ServerConfig) *processNode {
	t.Helper()

	binary := buildServerBinary(t)
	args := serverArgsFromConfig(cfg)
	cmd := exec.Command(binary, args...)
	cmd.Dir = repoRoot(t)
	logs := &lockedBuffer{}
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server process %s error = %v", cfg.NodeID, err)
	}
	return &processNode{cmd: cmd, logs: logs}
}

func (n *processNode) stop(t *testing.T) {
	t.Helper()

	if n == nil || n.cmd == nil || n.cmd.Process == nil {
		return
	}
	if n.cmd.ProcessState != nil && n.cmd.ProcessState.Exited() {
		return
	}
	if err := n.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("kill process error = %v", err)
	}
	_ = n.cmd.Wait()
}

func waitForProcessExitWithin(t *testing.T, node *processNode, timeout time.Duration) (int, string) {
	t.Helper()

	done := make(chan error, 1)
	go func() {
		done <- node.cmd.Wait()
	}()

	select {
	case err := <-done:
		exitCode := 0
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				t.Fatalf("process wait error = %v", err)
			}
		}
		return exitCode, node.logs.String()
	case <-time.After(timeout):
		node.stop(t)
		t.Fatalf("process did not exit within %s; logs:\n%s", timeout, node.logs.String())
		return 0, ""
	}
}

func waitForShardAssignmentWithin(t *testing.T, baseURL string, shardID shardmeta.ShardID, wantGroup string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		shards := controlGetJSON[model.ShardsResponse](t, baseURL+"/shards")
		for _, assignment := range shards.Assignments {
			if assignment.ShardID == uint32(shardID) && assignment.GroupID == wantGroup {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("shard %d did not move to %s within %s", shardID, wantGroup, timeout)
}

func waitForProcessHealthWithin(t *testing.T, node *processNode, addr string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if node.cmd.ProcessState != nil && node.cmd.ProcessState.Exited() {
			t.Fatalf("process for %s exited early; logs:\n%s", addr, node.logs.String())
		}
		resp, err := http.Get("http://" + addr + "/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("health check timed out for %s after %s; logs:\n%s", addr, timeout, node.logs.String())
}

func waitForMemberCountOnAnyWithin(t *testing.T, addrs []string, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, addr := range addrs {
			members := getMembers(t, addr)
			if len(members) == want {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	for _, addr := range addrs {
		members := getMembers(t, addr)
		if len(members) == want {
			return
		}
	}
	t.Fatalf("no node reported member count %d within %s", want, timeout)
}

func waitForMemberStatusOnAnyWithin(t *testing.T, addrs []string, nodeID string, predicate func(model.ClusterMember) bool, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, addr := range addrs {
			members := getMembers(t, addr)
			for _, member := range members {
				if member.ID == nodeID && predicate(member) {
					return
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	for _, addr := range addrs {
		members := getMembers(t, addr)
		for _, member := range members {
			if member.ID == nodeID && predicate(member) {
				return
			}
		}
	}
	t.Fatalf("member %s did not reach expected state on any node within %s", nodeID, timeout)
}

func waitForLeaderMemberOnAnyWithin(t *testing.T, addrs []string, allowedIDs []string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, addr := range addrs {
			members := getMembers(t, addr)
			for _, member := range members {
				if member.IsLeader && member.Online && slices.Contains(allowedIDs, member.ID) {
					return
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("no allowed leader %v was observed on any node within %s", allowedIDs, timeout)
}

func buildServerBinary(t *testing.T) string {
	t.Helper()

	buildServerBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ddb-server-bin-*")
		if err != nil {
			buildServerBinaryErr = err
			return
		}
		buildServerBinaryPath = filepath.Join(dir, "ddb-server")
		cmd := exec.Command("go", "build", "-o", buildServerBinaryPath, "./cmd/server")
		cmd.Dir = repoRoot(t)
		output, err := cmd.CombinedOutput()
		if err != nil {
			buildServerBinaryErr = errors.New(string(output))
			return
		}
	})
	if buildServerBinaryErr != nil {
		t.Fatalf("build server binary error = %v", buildServerBinaryErr)
	}
	return buildServerBinaryPath
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func serverArgsFromConfig(cfg config.ServerConfig) []string {
	args := []string{
		"--node-id", cfg.NodeID,
		"--http-addr", cfg.HTTPAddr,
		"--raft-addr", cfg.RaftAddr,
		"--raft-dir", cfg.RaftDir,
		"--db-path", cfg.DBPath,
	}
	if cfg.Bootstrap {
		args = append(args, "--bootstrap")
	}
	if cfg.Rejoin {
		args = append(args, "--rejoin")
	}
	if cfg.JoinAddr != "" {
		args = append(args, "--join", cfg.JoinAddr)
	}
	if len(cfg.ETCDEndpoints) > 0 {
		args = append(args, "--etcd", joinCSV(cfg.ETCDEndpoints))
	}
	if cfg.Role != "" {
		args = append(args, "--role", string(cfg.Role))
	}
	if cfg.GroupID != "" {
		args = append(args, "--group-id", cfg.GroupID)
	}
	if len(cfg.ControllerAddrs) > 0 {
		args = append(args, "--controller-addrs", joinCSV(cfg.ControllerAddrs))
	}
	return args
}

func joinCSV(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var out bytes.Buffer
	for i, item := range items {
		if i > 0 {
			out.WriteByte(',')
		}
		out.WriteString(item)
	}
	return out.String()
}

func moveShard(t *testing.T, baseURL string, shardID shardmeta.ShardID, groupID shardmeta.GroupID) {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"shard_id": shardID,
		"group_id": groupID,
	})
	if err != nil {
		t.Fatalf("json.Marshal(move-shard) error = %v", err)
	}
	resp, err := http.Post(baseURL+"/move-shard", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("http.Post(/move-shard) error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("/move-shard status=%d body=%s", resp.StatusCode, string(data))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
}

func buildSelectByID(table string, id int) string {
	return fmt.Sprintf("SELECT * FROM %s WHERE id = %d", table, id)
}

func groupContainsShard(groups []model.GroupStatus, groupID string, shardID uint32) bool {
	for _, group := range groups {
		if group.GroupID != groupID {
			continue
		}
		return slices.Contains(group.Shards, shardID)
	}
	return false
}
