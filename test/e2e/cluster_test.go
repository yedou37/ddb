package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"testing"
	"time"

	apppkg "github.com/yedou37/ddb/internal/app"
	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/discovery"
	"github.com/yedou37/ddb/internal/model"
	embedetcd "go.etcd.io/etcd/server/v3/embed"
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

func TestQuorumLossRejectsWrites(t *testing.T) {
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
	node2 := startNode(t, node2Cfg)
	waitForHealth(t, http2)
	node3 := startNode(t, node3Cfg)
	waitForHealth(t, http3)

	execSQL(t, http1, "CREATE TABLE quorum_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO quorum_books VALUES (1, 'before-quorum-loss')")
	waitForNamedRowCount(t, http2, "quorum_books", 1)
	waitForNamedRowCount(t, http3, "quorum_books", 1)

	node2.stop(t)
	node3.stop(t)

	status, response := waitForRejectedWrite(t, http1, "INSERT INTO quorum_books VALUES (2, 'should-fail')")
	if status == http.StatusOK && response.Success {
		t.Fatalf("write unexpectedly succeeded without quorum")
	}
	if got, ok := tryRowCount(http1, "quorum_books"); !ok || got != 1 {
		t.Fatalf("row count on surviving node = %d, ok=%t, want 1,true", got, ok)
	}
}

func TestRemovedFollowerCanRejoinAndCatchUp(t *testing.T) {
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

	execSQL(t, http1, "CREATE TABLE rejoin_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO rejoin_books VALUES (1, 'before-remove')")
	waitForNamedRowCount(t, http3, "rejoin_books", 1)

	postJSON(t, "http://"+http1+"/remove", model.RemoveRequest{NodeID: "node3"})
	waitForMemberCountWithin(t, http1, 2, 15*time.Second)
	node3.stop(t)

	execSQL(t, http1, "INSERT INTO rejoin_books VALUES (2, 'while-node3-removed')")
	waitForNamedRowCount(t, http2, "rejoin_books", 2)

	rejoinCfg := node3Cfg
	rejoinCfg.Rejoin = true
	_ = startNode(t, rejoinCfg)
	waitForHealthWithin(t, http3, 30*time.Second)
	waitForMemberCountWithin(t, http1, 3, 20*time.Second)
	waitForNamedRowCountWithin(t, http3, "rejoin_books", 2, 25*time.Second)

	execSQL(t, http1, "INSERT INTO rejoin_books VALUES (3, 'after-rejoin')")
	waitForNamedRowCount(t, http2, "rejoin_books", 3)
	waitForNamedRowCountWithin(t, http3, "rejoin_books", 3, 20*time.Second)
}

func TestDiscoveryAutoJoinWithoutExplicitJoinAddr(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	node1Cfg := config.ServerConfig{
		NodeID:        "node1",
		HTTPAddr:      http1,
		RaftAddr:      raft1,
		RaftDir:       filepath.Join(baseDir, "raft1"),
		DBPath:        filepath.Join(baseDir, "db1.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node2Cfg := config.ServerConfig{
		NodeID:        "node2",
		HTTPAddr:      http2,
		RaftAddr:      raft2,
		RaftDir:       filepath.Join(baseDir, "raft2"),
		DBPath:        filepath.Join(baseDir, "db2.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node3Cfg := config.ServerConfig{
		NodeID:        "node3",
		HTTPAddr:      http3,
		RaftAddr:      raft3,
		RaftDir:       filepath.Join(baseDir, "raft3"),
		DBPath:        filepath.Join(baseDir, "db3.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}

	_ = startNode(t, node1Cfg)
	waitForHealth(t, http1)
	waitForLeader(t, http1)

	_ = startNode(t, node2Cfg)
	waitForHealthWithin(t, http2, 30*time.Second)
	_ = startNode(t, node3Cfg)
	waitForHealthWithin(t, http3, 30*time.Second)

	waitForMemberCountWithin(t, http1, 3, 20*time.Second)

	client, err := discovery.New([]string{etcd.clientEndpoint})
	if err != nil {
		t.Fatalf("discovery.New() error = %v", err)
	}
	defer func() {
		_ = client.Close()
	}()
	waitForDiscoveryNodesWithin(t, client, 3, 20*time.Second)

	execSQL(t, http1, "CREATE TABLE autodiscovery_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO autodiscovery_books VALUES (1, 'registered')")
	waitForNamedRowCount(t, http2, "autodiscovery_books", 1)
	waitForNamedRowCount(t, http3, "autodiscovery_books", 1)
}

func TestMembersEndpointReflectsOfflineAndRemovedNodes(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	node1Cfg := config.ServerConfig{
		NodeID:        "node1",
		HTTPAddr:      http1,
		RaftAddr:      raft1,
		RaftDir:       filepath.Join(baseDir, "raft1"),
		DBPath:        filepath.Join(baseDir, "db1.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node2Cfg := config.ServerConfig{
		NodeID:        "node2",
		HTTPAddr:      http2,
		RaftAddr:      raft2,
		RaftDir:       filepath.Join(baseDir, "raft2"),
		DBPath:        filepath.Join(baseDir, "db2.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node3Cfg := config.ServerConfig{
		NodeID:        "node3",
		HTTPAddr:      http3,
		RaftAddr:      raft3,
		RaftDir:       filepath.Join(baseDir, "raft3"),
		DBPath:        filepath.Join(baseDir, "db3.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}

	_ = startNode(t, node1Cfg)
	waitForHealth(t, http1)
	waitForLeader(t, http1)
	_ = startNode(t, node2Cfg)
	waitForHealthWithin(t, http2, 30*time.Second)
	node3 := startNode(t, node3Cfg)
	waitForHealthWithin(t, http3, 30*time.Second)

	waitForMemberCountWithin(t, http1, 3, 20*time.Second)
	waitForMemberStatusWithin(t, http1, "node3", func(member model.ClusterMember) bool {
		return member.InRaft && member.Online && !member.Removed && member.Status == "online-voter"
	}, 20*time.Second)

	node3.stop(t)
	waitForMemberStatusWithin(t, http1, "node3", func(member model.ClusterMember) bool {
		return member.InRaft && !member.Online && !member.Removed && member.Status == "offline-voter"
	}, 20*time.Second)

	postJSON(t, "http://"+http1+"/remove", model.RemoveRequest{NodeID: "node3"})
	waitForMemberStatusWithin(t, http1, "node3", func(member model.ClusterMember) bool {
		return !member.InRaft && !member.Online && member.Removed && member.Status == "removed"
	}, 20*time.Second)
}

func TestAutoDiscoveryJoinAfterLeaderFailover(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)
	http4, raft4 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	node1Cfg := config.ServerConfig{
		NodeID:        "node1",
		HTTPAddr:      http1,
		RaftAddr:      raft1,
		RaftDir:       filepath.Join(baseDir, "raft1"),
		DBPath:        filepath.Join(baseDir, "db1.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node2Cfg := config.ServerConfig{
		NodeID:        "node2",
		HTTPAddr:      http2,
		RaftAddr:      raft2,
		RaftDir:       filepath.Join(baseDir, "raft2"),
		DBPath:        filepath.Join(baseDir, "db2.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node3Cfg := config.ServerConfig{
		NodeID:        "node3",
		HTTPAddr:      http3,
		RaftAddr:      raft3,
		RaftDir:       filepath.Join(baseDir, "raft3"),
		DBPath:        filepath.Join(baseDir, "db3.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node4Cfg := config.ServerConfig{
		NodeID:        "node4",
		HTTPAddr:      http4,
		RaftAddr:      raft4,
		RaftDir:       filepath.Join(baseDir, "raft4"),
		DBPath:        filepath.Join(baseDir, "db4.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}

	node1 := startNode(t, node1Cfg)
	waitForHealth(t, http1)
	waitForLeader(t, http1)
	_ = startNode(t, node2Cfg)
	waitForHealthWithin(t, http2, 30*time.Second)
	_ = startNode(t, node3Cfg)
	waitForHealthWithin(t, http3, 30*time.Second)
	waitForMemberCountWithin(t, http1, 3, 20*time.Second)

	client, err := discovery.New([]string{etcd.clientEndpoint})
	if err != nil {
		t.Fatalf("discovery.New() error = %v", err)
	}
	defer func() {
		_ = client.Close()
	}()
	waitForDiscoveryNodesWithin(t, client, 3, 20*time.Second)

	execSQL(t, http1, "CREATE TABLE failover_discovery_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO failover_discovery_books VALUES (1, 'before-failover')")
	waitForNamedRowCount(t, http2, "failover_discovery_books", 1)
	waitForNamedRowCount(t, http3, "failover_discovery_books", 1)

	node1.stop(t)
	waitForWriteSuccess(t, []string{http2, http3}, "INSERT INTO failover_discovery_books VALUES (2, 'after-failover')")
	newLeaderID := waitForDiscoveryLeaderWithin(t, client, []string{"node2", "node3"}, 20*time.Second)
	if newLeaderID == "" {
		t.Fatalf("expected a new discovery leader after failover")
	}

	_ = startNode(t, node4Cfg)
	waitForHealthWithin(t, http4, 30*time.Second)
	waitForMemberCountWithin(t, http2, 4, 20*time.Second)
	waitForNamedRowCountWithin(t, http4, "failover_discovery_books", 2, 25*time.Second)

	waitForWriteSuccess(t, []string{http2, http3, http4}, "INSERT INTO failover_discovery_books VALUES (3, 'after-node4-join')")
	waitForNamedRowCountWithin(t, http4, "failover_discovery_books", 3, 25*time.Second)
}

func TestReadAndMetadataEndpointsSurviveLeaderFailover(t *testing.T) {
	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	node1 := startNode(t, config.ServerConfig{
		NodeID:    "node1",
		HTTPAddr:  http1,
		RaftAddr:  raft1,
		RaftDir:   filepath.Join(baseDir, "raft1"),
		DBPath:    filepath.Join(baseDir, "db1.db"),
		Bootstrap: true,
	})
	waitForHealth(t, http1)
	waitForLeader(t, http1)
	_ = startNode(t, config.ServerConfig{
		NodeID:   "node2",
		HTTPAddr: http2,
		RaftAddr: raft2,
		RaftDir:  filepath.Join(baseDir, "raft2"),
		DBPath:   filepath.Join(baseDir, "db2.db"),
		JoinAddr: http1,
	})
	waitForHealth(t, http2)
	_ = startNode(t, config.ServerConfig{
		NodeID:   "node3",
		HTTPAddr: http3,
		RaftAddr: raft3,
		RaftDir:  filepath.Join(baseDir, "raft3"),
		DBPath:   filepath.Join(baseDir, "db3.db"),
		JoinAddr: http1,
	})
	waitForHealth(t, http3)

	execSQL(t, http1, "CREATE TABLE metadata_books (id INT PRIMARY KEY, name TEXT)")
	execSQL(t, http1, "INSERT INTO metadata_books VALUES (1, 'raft')")
	execSQL(t, http1, "INSERT INTO metadata_books VALUES (2, 'failover')")
	waitForNamedRowCount(t, http2, "metadata_books", 2)
	waitForNamedRowCount(t, http3, "metadata_books", 2)

	node1.stop(t)
	waitForReadableTableWithin(t, []string{http2, http3}, "metadata_books", 2, 20*time.Second)

	tables := waitForTablesWithin(t, []string{http2, http3}, 20*time.Second)
	if !slices.Contains(tables, "metadata_books") {
		t.Fatalf("tables = %#v, want metadata_books included", tables)
	}

	schema := waitForSchemaWithin(t, []string{http2, http3}, "metadata_books", 20*time.Second)
	if got, want := schema.Name, "metadata_books"; got != want {
		t.Fatalf("schema.Name = %q, want %q", got, want)
	}
	if got, want := schema.PrimaryKey, "id"; got != want {
		t.Fatalf("schema.PrimaryKey = %q, want %q", got, want)
	}

	status2 := getStatus(t, http2)
	status3 := getStatus(t, http3)
	if status2.Leader == "" && status3.Leader == "" {
		t.Fatalf("expected at least one surviving node to report a leader")
	}
}

func TestRemovedNodeCannotRestartWithoutRejoin(t *testing.T) {
	etcd := startEmbeddedEtcd(t)

	http1, raft1 := reserveAddr(t), reserveAddr(t)
	http2, raft2 := reserveAddr(t), reserveAddr(t)
	http3, raft3 := reserveAddr(t), reserveAddr(t)

	baseDir := t.TempDir()
	node1Cfg := config.ServerConfig{
		NodeID:        "node1",
		HTTPAddr:      http1,
		RaftAddr:      raft1,
		RaftDir:       filepath.Join(baseDir, "raft1"),
		DBPath:        filepath.Join(baseDir, "db1.db"),
		Bootstrap:     true,
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node2Cfg := config.ServerConfig{
		NodeID:        "node2",
		HTTPAddr:      http2,
		RaftAddr:      raft2,
		RaftDir:       filepath.Join(baseDir, "raft2"),
		DBPath:        filepath.Join(baseDir, "db2.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}
	node3Cfg := config.ServerConfig{
		NodeID:        "node3",
		HTTPAddr:      http3,
		RaftAddr:      raft3,
		RaftDir:       filepath.Join(baseDir, "raft3"),
		DBPath:        filepath.Join(baseDir, "db3.db"),
		ETCDEndpoints: []string{etcd.clientEndpoint},
	}

	_ = startNode(t, node1Cfg)
	waitForHealth(t, http1)
	waitForLeader(t, http1)
	_ = startNode(t, node2Cfg)
	waitForHealthWithin(t, http2, 30*time.Second)
	node3 := startNode(t, node3Cfg)
	waitForHealthWithin(t, http3, 30*time.Second)
	waitForMemberCountWithin(t, http1, 3, 20*time.Second)

	postJSON(t, "http://"+http1+"/remove", model.RemoveRequest{NodeID: "node3"})
	waitForMemberStatusWithin(t, http1, "node3", func(member model.ClusterMember) bool {
		return !member.InRaft && member.Removed && member.Status == "removed"
	}, 20*time.Second)

	node3.stop(t)

	_, err := apppkg.NewServerApp(node3Cfg)
	if !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("NewServerApp(removed node without rejoin) error = %v, want %v", err, http.ErrServerClosed)
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

func postJSON(t *testing.T, url string, payload any) {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal(%s) error = %v", url, err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http.Post(%s) error = %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("http.Post(%s) status=%d body=%s", url, resp.StatusCode, string(data))
	}
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

func waitForRejectedWrite(t *testing.T, addr, statement string) (int, model.SQLResponse) {
	t.Helper()

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		status, parsed := execSQLWithStatus(t, addr, statement)
		if status != http.StatusOK || !parsed.Success {
			return status, parsed
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("write %q unexpectedly kept succeeding on %s", statement, addr)
	return 0, model.SQLResponse{}
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

func tryTables(addr string) ([]string, bool) {
	resp, err := http.Get("http://" + addr + "/tables")
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var parsed model.SQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, false
	}
	if !parsed.Success {
		return nil, false
	}
	return parsed.Result.Tables, true
}

func trySchema(addr, table string) (model.TableSchema, bool) {
	resp, err := http.Get(fmt.Sprintf("http://%s/schema?table=%s", addr, table))
	if err != nil {
		return model.TableSchema{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return model.TableSchema{}, false
	}
	var schema model.TableSchema
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		return model.TableSchema{}, false
	}
	return schema, true
}

func getLeader(t *testing.T, addr string) model.NodeInfo {
	t.Helper()
	return getJSON[model.NodeInfo](t, fmt.Sprintf("http://%s/leader", addr))
}

func getStatus(t *testing.T, addr string) model.StatusResponse {
	t.Helper()
	return getJSON[model.StatusResponse](t, fmt.Sprintf("http://%s/status", addr))
}

func getMembers(t *testing.T, addr string) []model.ClusterMember {
	t.Helper()
	return getJSON[[]model.ClusterMember](t, fmt.Sprintf("http://%s/members", addr))
}

func waitForReadableTableWithin(t *testing.T, addrs []string, table string, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, addr := range addrs {
			if got, ok := tryRowCount(addr, table); ok && got == want {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("table %s was not readable with %d rows on any node within %s", table, want, timeout)
}

func waitForTablesWithin(t *testing.T, addrs []string, timeout time.Duration) []string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, addr := range addrs {
			if tables, ok := tryTables(addr); ok {
				return tables
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("/tables was not available on any node within %s", timeout)
	return nil
}

func waitForSchemaWithin(t *testing.T, addrs []string, table string, timeout time.Duration) model.TableSchema {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, addr := range addrs {
			if schema, ok := trySchema(addr, table); ok {
				return schema
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("/schema for %s was not available on any node within %s", table, timeout)
	return model.TableSchema{}
}

func waitForMemberCountWithin(t *testing.T, addr string, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		members := getMembers(t, addr)
		if len(members) == want {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	members := getMembers(t, addr)
	t.Fatalf("len(members) on %s = %d, want %d", addr, len(members), want)
}

func waitForDiscoveryNodesWithin(t *testing.T, client *discovery.Client, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		nodes, err := client.ListNodes(t.Context())
		if err == nil && len(nodes) == want {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}

	nodes, err := client.ListNodes(t.Context())
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	t.Fatalf("len(nodes) = %d, want %d", len(nodes), want)
}

func waitForDiscoveryLeaderWithin(t *testing.T, client *discovery.Client, allowedIDs []string, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		leader, err := client.FindLeader(t.Context())
		if err == nil && leader != nil && slices.Contains(allowedIDs, leader.ID) {
			return leader.ID
		}
		time.Sleep(150 * time.Millisecond)
	}
	return ""
}

func waitForMemberStatusWithin(t *testing.T, addr, nodeID string, predicate func(model.ClusterMember) bool, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		members := getMembers(t, addr)
		for _, member := range members {
			if member.ID == nodeID && predicate(member) {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}

	members := getMembers(t, addr)
	t.Fatalf("member %s on %s did not reach expected state, members=%+v", nodeID, addr, members)
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

type embeddedEtcd struct {
	server         *embedetcd.Etcd
	clientEndpoint string
}

func startEmbeddedEtcd(t *testing.T) *embeddedEtcd {
	t.Helper()

	clientAddr := reserveAddr(t)
	peerAddr := reserveAddr(t)
	clientURL, err := url.Parse("http://" + clientAddr)
	if err != nil {
		t.Fatalf("url.Parse(client) error = %v", err)
	}
	peerURL, err := url.Parse("http://" + peerAddr)
	if err != nil {
		t.Fatalf("url.Parse(peer) error = %v", err)
	}

	cfg := embedetcd.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.Name = "default"
	cfg.LogLevel = "error"
	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = []url.URL{*clientURL}
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = []url.URL{*peerURL}
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())

	server, err := embedetcd.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("StartEtcd() error = %v", err)
	}
	select {
	case <-server.Server.ReadyNotify():
	case <-time.After(15 * time.Second):
		server.Close()
		t.Fatalf("embedded etcd was not ready in time")
	}

	instance := &embeddedEtcd{
		server:         server,
		clientEndpoint: clientAddr,
	}
	t.Cleanup(func() {
		server.Close()
	})
	return instance
}
