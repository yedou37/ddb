package router

import (
	"testing"

	"github.com/yedou37/ddb/internal/shardmeta"
)

func testConfig() shardmeta.ClusterConfig {
	return shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	})
}

func TestRouteIsStableForSameKey(t *testing.T) {
	instance, err := New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first, err := instance.Route("users", 42, testConfig())
	if err != nil {
		t.Fatalf("Route(first) error = %v", err)
	}
	second, err := instance.Route("users", 42, testConfig())
	if err != nil {
		t.Fatalf("Route(second) error = %v", err)
	}
	if first != second {
		t.Fatalf("Route() = %#v then %#v, want stable result", first, second)
	}
}

func TestRouteSpreadsAcrossMultipleShards(t *testing.T) {
	instance, err := New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	seen := make(map[shardmeta.ShardID]bool)
	for key := 0; key < 64; key++ {
		result, err := instance.Route("users", key, testConfig())
		if err != nil {
			t.Fatalf("Route(%d) error = %v", key, err)
		}
		seen[result.ShardID] = true
	}
	if got := len(seen); got < 4 {
		t.Fatalf("len(seen shards) = %d, want at least 4", got)
	}
}

func TestRouteReturnsGroupFromConfig(t *testing.T) {
	instance, err := New(shardmeta.DefaultTotalShards)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := instance.Route("orders", 7, testConfig())
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if result.GroupID == "" {
		t.Fatalf("result.GroupID = empty, want non-empty")
	}
}
