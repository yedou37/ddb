package router

import (
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"

	"github.com/yedou37/ddb/internal/shardmeta"
)

const defaultVirtualNodes = 32

type RouteResult struct {
	ShardID shardmeta.ShardID
	GroupID shardmeta.GroupID
}

type Router struct {
	points []ringPoint
}

type ringPoint struct {
	hash    uint32
	shardID shardmeta.ShardID
}

func New(totalShards int) (*Router, error) {
	if totalShards <= 0 {
		return nil, errors.New("totalShards must be greater than 0")
	}

	points := make([]ringPoint, 0, totalShards*defaultVirtualNodes)
	for shardIndex := 0; shardIndex < totalShards; shardIndex++ {
		shardID := shardmeta.ShardID(shardIndex)
		for replica := 0; replica < defaultVirtualNodes; replica++ {
			points = append(points, ringPoint{
				hash:    hashValue("shard:" + strconv.Itoa(shardIndex) + ":" + strconv.Itoa(replica)),
				shardID: shardID,
			})
		}
	}
	slices.SortFunc(points, func(a, b ringPoint) int {
		switch {
		case a.hash < b.hash:
			return -1
		case a.hash > b.hash:
			return 1
		default:
			return 0
		}
	})

	return &Router{points: points}, nil
}

func (r *Router) Route(table string, primaryKey any, config shardmeta.ClusterConfig) (RouteResult, error) {
	if r == nil || len(r.points) == 0 {
		return RouteResult{}, errors.New("router is not initialized")
	}
	if err := config.Validate(); err != nil {
		return RouteResult{}, err
	}
	if table == "" {
		return RouteResult{}, errors.New("table is required")
	}

	hash := hashValue(fmt.Sprintf("%s:%v", table, primaryKey))
	index := slices.IndexFunc(r.points, func(point ringPoint) bool {
		return point.hash >= hash
	})
	if index == -1 {
		index = 0
	}

	shardID := r.points[index].shardID
	groupID, ok := config.GroupForShard(shardID)
	if !ok {
		return RouteResult{}, fmt.Errorf("group not found for shard %d", shardID)
	}
	return RouteResult{ShardID: shardID, GroupID: groupID}, nil
}

func hashValue(value string) uint32 {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(value))
	return hasher.Sum32()
}
