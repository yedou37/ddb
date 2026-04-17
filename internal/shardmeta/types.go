package shardmeta

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/yedou37/ddb/internal/model"
)

const DefaultTotalShards = 8

type NodeRole string

const (
	RoleShardNode  NodeRole = "shard"
	RoleController NodeRole = "controller"
	RoleAPIServer  NodeRole = "apiserver"
)

func (r NodeRole) OrDefault() NodeRole {
	switch r {
	case RoleShardNode, RoleController, RoleAPIServer:
		return r
	default:
		return RoleShardNode
	}
}

type ShardID uint32
type GroupID string

type ShardAssignment struct {
	ShardID ShardID `json:"shard_id"`
	GroupID GroupID `json:"group_id"`
}

type GroupInfo struct {
	ID    GroupID          `json:"id"`
	Nodes []model.NodeInfo `json:"nodes,omitempty"`
}

type ClusterConfig struct {
	Version     uint64            `json:"version"`
	TotalShards int               `json:"total_shards"`
	Assignments []ShardAssignment `json:"assignments"`
	Groups      []GroupInfo       `json:"groups,omitempty"`
}

func NewClusterConfig(totalShards int, assignments map[ShardID]GroupID) ClusterConfig {
	keys := make([]int, 0, len(assignments))
	for shardID := range assignments {
		keys = append(keys, int(shardID))
	}
	slices.Sort(keys)

	config := ClusterConfig{
		Version:     1,
		TotalShards: totalShards,
		Assignments: make([]ShardAssignment, 0, len(assignments)),
	}
	for _, key := range keys {
		config.Assignments = append(config.Assignments, ShardAssignment{
			ShardID: ShardID(key),
			GroupID: assignments[ShardID(key)],
		})
	}
	return config
}

func (c ClusterConfig) Validate() error {
	if c.TotalShards <= 0 {
		return errors.New("total_shards must be greater than 0")
	}
	if len(c.Assignments) != c.TotalShards {
		return fmt.Errorf("assignments count = %d, want %d", len(c.Assignments), c.TotalShards)
	}

	seen := make(map[ShardID]bool, len(c.Assignments))
	for _, assignment := range c.Assignments {
		if int(assignment.ShardID) >= c.TotalShards {
			return fmt.Errorf("shard_id %d out of range", assignment.ShardID)
		}
		if assignment.GroupID == "" {
			return fmt.Errorf("shard %d has empty group id", assignment.ShardID)
		}
		if seen[assignment.ShardID] {
			return fmt.Errorf("duplicate shard_id %d", assignment.ShardID)
		}
		seen[assignment.ShardID] = true
	}
	return nil
}

func (c ClusterConfig) GroupForShard(shardID ShardID) (GroupID, bool) {
	for _, assignment := range c.Assignments {
		if assignment.ShardID == shardID {
			return assignment.GroupID, true
		}
	}
	return "", false
}

func (c ClusterConfig) WithVersion(version uint64) ClusterConfig {
	c.Version = version
	return c
}

func (c ClusterConfig) MarshalJSON() ([]byte, error) {
	type alias ClusterConfig
	return json.Marshal(alias(c))
}
