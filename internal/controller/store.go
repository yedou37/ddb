package controller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/yedou37/ddb/internal/shardmeta"
)

var ErrConfigNotFound = errors.New("controller config not found")

type ConfigStore interface {
	Load(ctx context.Context) (shardmeta.ClusterConfig, error)
	Save(ctx context.Context, config shardmeta.ClusterConfig) error
}

type MemoryStore struct {
	mu     sync.RWMutex
	config *shardmeta.ClusterConfig
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Load(context.Context) (shardmeta.ClusterConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config == nil {
		return shardmeta.ClusterConfig{}, ErrConfigNotFound
	}
	return *s.config, nil
}

func (s *MemoryStore) Save(_ context.Context, config shardmeta.ClusterConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyConfig := config
	s.config = &copyConfig
	return nil
}

type FileStore struct {
	path string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load(_ context.Context) (shardmeta.ClusterConfig, error) {
	if s == nil || s.path == "" {
		return shardmeta.ClusterConfig{}, ErrConfigNotFound
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return shardmeta.ClusterConfig{}, ErrConfigNotFound
	}
	if err != nil {
		return shardmeta.ClusterConfig{}, err
	}

	var config shardmeta.ClusterConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return shardmeta.ClusterConfig{}, err
	}
	return config, nil
}

func (s *FileStore) Save(_ context.Context, config shardmeta.ClusterConfig) error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

type ChainStore struct {
	stores []ConfigStore
}

func NewChainStore(stores ...ConfigStore) *ChainStore {
	filtered := make([]ConfigStore, 0, len(stores))
	for _, store := range stores {
		if store != nil {
			filtered = append(filtered, store)
		}
	}
	return &ChainStore{stores: filtered}
}

func (s *ChainStore) Load(ctx context.Context) (shardmeta.ClusterConfig, error) {
	if s == nil || len(s.stores) == 0 {
		return shardmeta.ClusterConfig{}, ErrConfigNotFound
	}

	var lastNotFound bool
	for _, store := range s.stores {
		config, err := store.Load(ctx)
		if err == nil {
			return config, nil
		}
		if errors.Is(err, ErrConfigNotFound) {
			lastNotFound = true
			continue
		}
		return shardmeta.ClusterConfig{}, err
	}
	if lastNotFound {
		return shardmeta.ClusterConfig{}, ErrConfigNotFound
	}
	return shardmeta.ClusterConfig{}, ErrConfigNotFound
}

func (s *ChainStore) Save(ctx context.Context, config shardmeta.ClusterConfig) error {
	if s == nil {
		return nil
	}
	for _, store := range s.stores {
		if err := store.Save(ctx, config); err != nil {
			return err
		}
	}
	return nil
}
