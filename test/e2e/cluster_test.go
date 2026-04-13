package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	apppkg "github.com/yedou37/ddb/internal/app"
	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/model"
)

type runningNode struct {
	app   *apppkg.App
	cfg   config.ServerConfig
	errCh chan error
}

func TestThreeNodeClusterReplicatesWrites(t *testing.T) {
	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	node1 := startNode(t, config.ServerConfig{
		NodeID:    "node1",
		HTTPAddr:  http1,
		RaftAddr:  raft1,
		RaftDir:   filepath.Join(t.TempDir(), "raft1"),
		DBPath:    filepath.Join(t.TempDir(), "db1.db"),
		Bootstrap: true,
	})
	waitForHealth(t, http1)
	waitForLeader(t, http1)

	_ = startNode(t, config.ServerConfig{
		NodeID:   "node2",
		HTTPAddr: http2,
		RaftAddr: raft2,
		RaftDir:  filepath.Join(t.TempDir(), "raft2"),
		DBPath:   filepath.Join(t.TempDir(), "db2.db"),
		JoinAddr: http1,
	})
	waitForHealth(t, http2)

	_ = startNode(t, config.ServerConfig{
		NodeID:   "node3",
		HTTPAddr: http3,
		RaftAddr: raft3,
		RaftDir:  filepath.Join(t.TempDir(), "raft3"),
		DBPath:   filepath.Join(t.TempDir(), "db3.db"),
		JoinAddr: http1,
	})
	waitForHealth(t, http3)

	execSQL(t, http1, "CREATE TABLE books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO books VALUES (1, 'raft')")
	execSQL(t, http1, "INSERT INTO books VALUES (2, 'follower')")

	waitForRowCount(t, http2, 2)
	waitForRowCount(t, http3, 2)

	leader := getLeader(t, http2)
	if leader.ID != "node1" {
		t.Fatalf("leader.ID = %q, want node1", leader.ID)
	}

	status := getStatus(t, http3)
	if status.Role == "" {
		t.Fatalf("status.Role = empty, want non-empty")
	}

	_ = node1
}

func TestLeaderFailoverKeepsClusterWritable(t *testing.T) {
	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	node1 := startNode(t, config.ServerConfig{
		NodeID:    "node1",
		HTTPAddr:  http1,
		RaftAddr:  raft1,
		RaftDir:   filepath.Join(t.TempDir(), "raft1"),
		DBPath:    filepath.Join(t.TempDir(), "db1.db"),
		Bootstrap: true,
	})
	waitForHealth(t, http1)
	waitForLeader(t, http1)

	_ = startNode(t, config.ServerConfig{
		NodeID:   "node2",
		HTTPAddr: http2,
		RaftAddr: raft2,
		RaftDir:  filepath.Join(t.TempDir(), "raft2"),
		DBPath:   filepath.Join(t.TempDir(), "db2.db"),
		JoinAddr: http1,
	})
	waitForHealth(t, http2)

	_ = startNode(t, config.ServerConfig{
		NodeID:   "node3",
		HTTPAddr: http3,
		RaftAddr: raft3,
		RaftDir:  filepath.Join(t.TempDir(), "raft3"),
		DBPath:   filepath.Join(t.TempDir(), "db3.db"),
		JoinAddr: http1,
	})
	waitForHealth(t, http3)

	execSQL(t, http1, "CREATE TABLE failover_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO failover_books VALUES (1, 'before-failover')")
	waitForNamedRowCount(t, http2, "failover_books", 1)
	waitForNamedRowCount(t, http3, "failover_books", 1)

	node1.stop(t)
	waitForWriteSuccess(t, []string{http2, http3}, "INSERT INTO failover_books VALUES (2, 'after-failover')")
	waitForNamedRowCount(t, http2, "failover_books", 2)
	waitForNamedRowCount(t, http3, "failover_books", 2)
}

func TestFollowerRestartCatchesUpMissingWrites(t *testing.T) {
	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	node1Cfg := config.ServerConfig{
		NodeID:    "node1",
		HTTPAddr:  http1,
		RaftAddr:  raft1,
		RaftDir:   filepath.Join(baseDir, "raft1"),
		DBPath:    filepath.Join(baseDir, "db1.db"),
		Bootstrap: true,
	}
	node2Cfg := config.ServerConfig{
		NodeID:   "node2",
		HTTPAddr: http2,
		RaftAddr: raft2,
		RaftDir:  filepath.Join(baseDir, "raft2"),
		DBPath:   filepath.Join(baseDir, "db2.db"),
		JoinAddr: http1,
	}
	node3Cfg := config.ServerConfig{
		NodeID:   "node3",
		HTTPAddr: http3,
		RaftAddr: raft3,
		RaftDir:  filepath.Join(baseDir, "raft3"),
		DBPath:   filepath.Join(baseDir, "db3.db"),
		JoinAddr: http1,
	}

	_ = startNode(t, node1Cfg)
	waitForHealth(t, http1)
	waitForLeader(t, http1)

	_ = startNode(t, node2Cfg)
	waitForHealth(t, http2)

	node3 := startNode(t, node3Cfg)
	waitForHealth(t, http3)

	execSQL(t, http1, "CREATE TABLE restart_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO restart_books VALUES (1, 'first')")
	waitForNamedRowCount(t, http3, "restart_books", 1)

	node3.stop(t)
	execSQL(t, http1, "INSERT INTO restart_books VALUES (2, 'second')")
	waitForNamedRowCount(t, http2, "restart_books", 2)

	_ = startNode(t, node3Cfg)
	waitForHealthWithin(t, http3, 30*time.Second)
	waitForNamedRowCountWithin(t, http3, "restart_books", 2, 25*time.Second)
}

func TestFollowerWriteReturnsLeaderRedirect(t *testing.T) {
	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	_ = startNode(t, config.ServerConfig{
		NodeID:    "node1",
		HTTPAddr:  http1,
		RaftAddr:  raft1,
		RaftDir:   filepath.Join(t.TempDir(), "raft1"),
		DBPath:    filepath.Join(t.TempDir(), "db1.db"),
		Bootstrap: true,
	})
	waitForHealth(t, http1)
	waitForLeader(t, http1)

	_ = startNode(t, config.ServerConfig{
		NodeID:   "node2",
		HTTPAddr: http2,
		RaftAddr: raft2,
		RaftDir:  filepath.Join(t.TempDir(), "raft2"),
		DBPath:   filepath.Join(t.TempDir(), "db2.db"),
		JoinAddr: http1,
	})
	waitForHealth(t, http2)

	_ = startNode(t, config.ServerConfig{
		NodeID:   "node3",
		HTTPAddr: http3,
		RaftAddr: raft3,
		RaftDir:  filepath.Join(t.TempDir(), "raft3"),
		DBPath:   filepath.Join(t.TempDir(), "db3.db"),
		JoinAddr: http1,
	})
	waitForHealth(t, http3)

	execSQL(t, http1, "CREATE TABLE redirect_books (id INT PRIMARY KEY, name TEXT)")

	status, response := execSQLWithStatus(t, http2, "INSERT INTO redirect_books VALUES (1, 'from-follower')")
	if status != http.StatusConflict {
		t.Fatalf("follower write status = %d, want %d", status, http.StatusConflict)
	}
	if response.Success {
		t.Fatalf("follower write response.Success = true, want false")
	}
	if got, want := response.Leader, "http://"+http1; got != want {
		t.Fatalf("follower write leader = %q, want %q", got, want)
	}

	waitForNamedRowCount(t, http1, "redirect_books", 0)

	execSQL(t, http1, "INSERT INTO redirect_books VALUES (1, 'from-leader')")
	waitForNamedRowCount(t, http2, "redirect_books", 1)
	waitForNamedRowCount(t, http3, "redirect_books", 1)
}

func TestDeleteReplicatesAcrossFollowers(t *testing.T) {
	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	_ = startNode(t, config.ServerConfig{
		NodeID:    "node1",
		HTTPAddr:  http1,
		RaftAddr:  raft1,
		RaftDir:   filepath.Join(t.TempDir(), "raft1"),
		DBPath:    filepath.Join(t.TempDir(), "db1.db"),
		Bootstrap: true,
	})
	waitForHealth(t, http1)
	waitForLeader(t, http1)

	_ = startNode(t, config.ServerConfig{
		NodeID:   "node2",
		HTTPAddr: http2,
		RaftAddr: raft2,
		RaftDir:  filepath.Join(t.TempDir(), "raft2"),
		DBPath:   filepath.Join(t.TempDir(), "db2.db"),
		JoinAddr: http1,
	})
	waitForHealth(t, http2)

	_ = startNode(t, config.ServerConfig{
		NodeID:   "node3",
		HTTPAddr: http3,
		RaftAddr: raft3,
		RaftDir:  filepath.Join(t.TempDir(), "raft3"),
		DBPath:   filepath.Join(t.TempDir(), "db3.db"),
		JoinAddr: http1,
	})
	waitForHealth(t, http3)

	execSQL(t, http1, "CREATE TABLE delete_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO delete_books VALUES (1, 'first')")
	execSQL(t, http1, "INSERT INTO delete_books VALUES (2, 'second')")
	waitForNamedRowCount(t, http2, "delete_books", 2)
	waitForNamedRowCount(t, http3, "delete_books", 2)

	execSQL(t, http1, "DELETE FROM delete_books WHERE id = 1")
	waitForNamedRowCount(t, http2, "delete_books", 1)
	waitForNamedRowCount(t, http3, "delete_books", 1)

	result := execSQL(t, http2, "SELECT * FROM delete_books")
	if got, want := len(result.Result.Rows), 1; got != want {
		t.Fatalf("len(result.Result.Rows) = %d, want %d", got, want)
	}
	if got, want := result.Result.Rows[0][0], float64(2); got != want {
		t.Fatalf("remaining row primary key = %#v, want %#v", got, want)
	}
}

func startNode(t *testing.T, cfg config.ServerConfig) *runningNode {
	t.Helper()

	instance, err := apppkg.NewServerApp(cfg)
	if err != nil {
		t.Fatalf("NewServerApp(%s) error = %v", cfg.NodeID, err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- instance.Run()
	}()

	node := &runningNode{
		app:   instance,
		cfg:   cfg,
		errCh: errCh,
	}

	t.Cleanup(func() {
		node.stop(t)
	})

	return node
}

func reserveAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}

func waitForHealth(t *testing.T, addr string) {
	t.Helper()
	waitForHealthWithin(t, addr, 10*time.Second)
}

func waitForHealthWithin(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("health check timed out for %s after %s", addr, timeout)
}

func waitForLeader(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/leader")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("leader election timed out for %s", addr)
}

func execSQL(t *testing.T, addr, statement string) model.SQLResponse {
	t.Helper()

	status, parsed := execSQLWithStatus(t, addr, statement)
	if status != http.StatusOK || !parsed.Success {
		data, _ := json.Marshal(parsed)
		t.Fatalf("SQL %q failed: status=%d body=%s", statement, status, string(data))
	}
	return parsed
}

func execSQLWithStatus(t *testing.T, addr, statement string) (int, model.SQLResponse) {
	t.Helper()

	body, err := json.Marshal(model.SQLRequest{SQL: statement})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	resp, err := http.Post("http://"+addr+"/sql", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http.Post(%s) error = %v", statement, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}

	var parsed model.SQLResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return resp.StatusCode, parsed
}

func waitForRowCount(t *testing.T, addr string, want int) {
	t.Helper()
	waitForNamedRowCount(t, addr, "books", want)
}

func waitForNamedRowCount(t *testing.T, addr, table string, want int) {
	t.Helper()
	waitForNamedRowCountWithin(t, addr, table, want, 5*time.Second)
}

func waitForNamedRowCountWithin(t *testing.T, addr, table string, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got, ok := tryRowCount(addr, table)
		if ok && got == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	got, ok := tryRowCount(addr, table)
	if !ok {
		t.Fatalf("table %s on %s was not queryable within timeout", table, addr)
	}
	t.Fatalf("len(rows) on %s = %d, want %d", addr, got, want)
}

func waitForWriteSuccess(t *testing.T, addrs []string, statement string) {
	t.Helper()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		for _, addr := range addrs {
			if tryExecSQL(addr, statement) {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("no node accepted write %q within timeout", statement)
}

func tryExecSQL(addr, statement string) bool {
	body, err := json.Marshal(model.SQLRequest{SQL: statement})
	if err != nil {
		return false
	}

	resp, err := http.Post("http://"+addr+"/sql", "application/json", bytes.NewReader(body))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var parsed model.SQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return false
	}
	return parsed.Success
}

func tryRowCount(addr, table string) (int, bool) {
	body, err := json.Marshal(model.SQLRequest{SQL: "SELECT * FROM " + table})
	if err != nil {
		return 0, false
	}

	resp, err := http.Post("http://"+addr+"/sql", "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	var parsed model.SQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, false
	}
	if !parsed.Success {
		return 0, false
	}
	return len(parsed.Result.Rows), true
}

func getLeader(t *testing.T, addr string) model.NodeInfo {
	t.Helper()
	return getJSON[model.NodeInfo](t, fmt.Sprintf("http://%s/leader", addr))
}

func getStatus(t *testing.T, addr string) model.StatusResponse {
	t.Helper()
	return getJSON[model.StatusResponse](t, fmt.Sprintf("http://%s/status", addr))
}

func getJSON[T any](t *testing.T, url string) T {
	t.Helper()

	var result T
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("http.Get(%s) error = %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("http.Get(%s) status=%d body=%s", url, resp.StatusCode, string(data))
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("json.Decode(%s) error = %v", url, err)
	}
	return result
}

func (n *runningNode) stop(t *testing.T) {
	t.Helper()
	if n == nil || n.app == nil {
		return
	}
	_ = n.app.Close()
	select {
	case err := <-n.errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Run(%s) error = %v", n.cfg.NodeID, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting node %s to stop", n.cfg.NodeID)
	}
	n.app = nil
}
