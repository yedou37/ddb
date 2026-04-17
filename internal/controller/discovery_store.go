package controller

import (
	"context"

	"github.com/yedou37/ddb/internal/discovery"
	"github.com/yedou37/ddb/internal/shardmeta"
)

type DiscoveryStore struct {
	client *discovery.Client
}

func NewDiscoveryStore(client *discovery.Client) *DiscoveryStore {
	return &DiscoveryStore{client: client}
}

func (s *DiscoveryStore) Load(ctx context.Context) (shardmeta.ClusterConfig, error) {
	if s == nil || s.client == nil {
		return shardmeta.ClusterConfig{}, ErrConfigNotFound
	}
	config, err := s.client.LoadControllerConfig(ctx)
	if err != nil {
		if err == discovery.ErrControllerConfigNotFound {
			return shardmeta.ClusterConfig{}, ErrConfigNotFound
		}
		return shardmeta.ClusterConfig{}, err
	}
	return config, nil
}

func (s *DiscoveryStore) Save(ctx context.Context, config shardmeta.ClusterConfig) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.SaveControllerConfig(ctx, config)
}
