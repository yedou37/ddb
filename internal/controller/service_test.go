package controller

import (
	"testing"

	"github.com/yedou37/ddb/internal/shardmeta"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService(shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestMoveShard(t *testing.T) {
	service := newTestService(t)

	config, err := service.MoveShard(6, "g3")
	if err != nil {
		t.Fatalf("MoveShard() error = %v", err)
	}
	groupID, ok := config.GroupForShard(6)
	if !ok {
		t.Fatalf("GroupForShard(6) ok = false, want true")
	}
	if got, want := groupID, shardmeta.GroupID("g3"); got != want {
		t.Fatalf("GroupForShard(6) = %q, want %q", got, want)
	}
	if got, want := config.Version, uint64(2); got != want {
		t.Fatalf("config.Version = %d, want %d", got, want)
	}
}

func TestRebalance(t *testing.T) {
	service := newTestService(t)

	config, err := service.Rebalance([]shardmeta.GroupID{"g1", "g2", "g3"})
	if err != nil {
		t.Fatalf("Rebalance() error = %v", err)
	}
	if got, want := len(config.Assignments), shardmeta.DefaultTotalShards; got != want {
		t.Fatalf("len(config.Assignments) = %d, want %d", got, want)
	}
	if got, want := config.Assignments[2].GroupID, shardmeta.GroupID("g3"); got != want {
		t.Fatalf("config.Assignments[2].GroupID = %q, want %q", got, want)
	}
}

func TestSharedStorePropagatesConfigAcrossServices(t *testing.T) {
	store := NewMemoryStore()
	service1, err := NewBootstrapService(shardmeta.DefaultTotalShards, []shardmeta.GroupID{"g1", "g2"}, store)
	if err != nil {
		t.Fatalf("NewBootstrapService(service1) error = %v", err)
	}
	service2, err := NewBootstrapService(shardmeta.DefaultTotalShards, []shardmeta.GroupID{"g1", "g2"}, store)
	if err != nil {
		t.Fatalf("NewBootstrapService(service2) error = %v", err)
	}

	updated, err := service1.MoveShard(6, "g3")
	if err != nil {
		t.Fatalf("service1.MoveShard() error = %v", err)
	}
	if got, want := updated.Version, uint64(2); got != want {
		t.Fatalf("updated.Version = %d, want %d", got, want)
	}

	config2 := service2.CurrentConfig()
	groupID, ok := config2.GroupForShard(6)
	if !ok {
		t.Fatalf("service2.CurrentConfig().GroupForShard(6) ok = false, want true")
	}
	if got, want := groupID, shardmeta.GroupID("g3"); got != want {
		t.Fatalf("service2 shard 6 group = %q, want %q", got, want)
	}
}
