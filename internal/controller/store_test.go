package controller

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yedou37/ddb/internal/shardmeta"
)

func TestFileStorePersistsConfig(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "controller.json"))
	original := shardmeta.NewClusterConfig(shardmeta.DefaultTotalShards, map[shardmeta.ShardID]shardmeta.GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}).WithVersion(5)

	if err := store.Save(context.Background(), original); err != nil {
		t.Fatalf("store.Save() error = %v", err)
	}

	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("store.Load() error = %v", err)
	}
	if got, want := loaded.Version, uint64(5); got != want {
		t.Fatalf("loaded.Version = %d, want %d", got, want)
	}
	groupID, ok := loaded.GroupForShard(7)
	if !ok {
		t.Fatalf("loaded.GroupForShard(7) ok = false, want true")
	}
	if got, want := groupID, shardmeta.GroupID("g2"); got != want {
		t.Fatalf("loaded.GroupForShard(7) = %q, want %q", got, want)
	}
}
