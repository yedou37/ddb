package shardmeta

import (
	"encoding/json"
	"testing"
)

func TestClusterConfigValidateAndGroupLookup(t *testing.T) {
	config := NewClusterConfig(DefaultTotalShards, map[ShardID]GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	})

	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	groupID, ok := config.GroupForShard(6)
	if !ok {
		t.Fatalf("GroupForShard(6) ok = false, want true")
	}
	if got, want := groupID, GroupID("g2"); got != want {
		t.Fatalf("GroupForShard(6) = %q, want %q", got, want)
	}
}

func TestClusterConfigJSONRoundTrip(t *testing.T) {
	config := NewClusterConfig(DefaultTotalShards, map[ShardID]GroupID{
		0: "g1", 1: "g1", 2: "g1", 3: "g1",
		4: "g2", 5: "g2", 6: "g2", 7: "g2",
	}).WithVersion(3)

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded ClusterConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := decoded.Version, uint64(3); got != want {
		t.Fatalf("decoded.Version = %d, want %d", got, want)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("decoded.Validate() error = %v", err)
	}
}
