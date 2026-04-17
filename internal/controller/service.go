package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/yedou37/ddb/internal/shardmeta"
)

var ErrShardNotFound = errors.New("shard not found")
var ErrShardMigrationInProgress = errors.New("shard migration in progress")

type ShardMigrationError struct {
	ShardID shardmeta.ShardID
}

func (e ShardMigrationError) Error() string {
	return fmt.Sprintf("shard %d is migrating, retry later", e.ShardID)
}

func (e ShardMigrationError) Unwrap() error {
	return ErrShardMigrationInProgress
}

type Service struct {
	mu             sync.RWMutex
	store          ConfigStore
	config         shardmeta.ClusterConfig
	lockedShardIDs map[shardmeta.ShardID]struct{}
}

func NewBootstrapService(totalShards int, groupIDs []shardmeta.GroupID, store ConfigStore) (*Service, error) {
	if totalShards <= 0 {
		totalShards = shardmeta.DefaultTotalShards
	}
	if len(groupIDs) == 0 {
		groupIDs = []shardmeta.GroupID{"g1", "g2"}
	}

	if store == nil {
		store = NewMemoryStore()
	}

	ctx := context.Background()
	if config, err := store.Load(ctx); err == nil {
		if validateErr := config.Validate(); validateErr != nil {
			return nil, validateErr
		}
		return &Service{store: store, config: config, lockedShardIDs: make(map[shardmeta.ShardID]struct{})}, nil
	} else if !errors.Is(err, ErrConfigNotFound) {
		return nil, err
	}

	assignments := make(map[shardmeta.ShardID]shardmeta.GroupID, totalShards)
	for shardIndex := 0; shardIndex < totalShards; shardIndex++ {
		assignments[shardmeta.ShardID(shardIndex)] = groupIDs[shardIndex%len(groupIDs)]
	}

	config := shardmeta.NewClusterConfig(totalShards, assignments)
	if err := store.Save(ctx, config); err != nil {
		return nil, err
	}
	return &Service{store: store, config: config, lockedShardIDs: make(map[shardmeta.ShardID]struct{})}, nil
}

func NewService(config shardmeta.ClusterConfig) (*Service, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Service{store: NewMemoryStore(), config: config, lockedShardIDs: make(map[shardmeta.ShardID]struct{})}, nil
}

func (s *Service) CurrentConfig() shardmeta.ClusterConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshLocked(context.Background())
	return s.config
}

func (s *Service) MoveShard(shardID shardmeta.ShardID, groupID shardmeta.GroupID) (shardmeta.ClusterConfig, error) {
	config, err := s.PreviewMoveShard(shardID, groupID)
	if err != nil {
		return shardmeta.ClusterConfig{}, err
	}
	return s.UpdateConfig(config)
}

func (s *Service) PreviewMoveShard(shardID shardmeta.ShardID, groupID shardmeta.GroupID) (shardmeta.ClusterConfig, error) {
	if groupID == "" {
		return shardmeta.ClusterConfig{}, errors.New("group id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return shardmeta.ClusterConfig{}, err
	}

	assignments := slices.Clone(s.config.Assignments)
	updated := false
	for index, assignment := range assignments {
		if assignment.ShardID == shardID {
			assignments[index].GroupID = groupID
			updated = true
			break
		}
	}
	if !updated {
		return shardmeta.ClusterConfig{}, fmt.Errorf("%w: %d", ErrShardNotFound, shardID)
	}

	next := s.config
	next.Assignments = assignments
	next.Version++
	return next, nil
}

func (s *Service) Rebalance(groupIDs []shardmeta.GroupID) (shardmeta.ClusterConfig, error) {
	config, err := s.PreviewRebalance(groupIDs)
	if err != nil {
		return shardmeta.ClusterConfig{}, err
	}
	return s.UpdateConfig(config)
}

func (s *Service) PreviewRebalance(groupIDs []shardmeta.GroupID) (shardmeta.ClusterConfig, error) {
	if len(groupIDs) == 0 {
		return shardmeta.ClusterConfig{}, errors.New("at least one group id is required")
	}

	normalized := make([]shardmeta.GroupID, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID == "" {
			return shardmeta.ClusterConfig{}, errors.New("group id is required")
		}
		normalized = append(normalized, groupID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(context.Background()); err != nil {
		return shardmeta.ClusterConfig{}, err
	}

	assignments := make([]shardmeta.ShardAssignment, 0, s.config.TotalShards)
	for shardIndex := 0; shardIndex < s.config.TotalShards; shardIndex++ {
		assignments = append(assignments, shardmeta.ShardAssignment{
			ShardID: shardmeta.ShardID(shardIndex),
			GroupID: normalized[shardIndex%len(normalized)],
		})
	}
	next := s.config
	next.Assignments = assignments
	next.Version++
	return next, nil
}

func (s *Service) UpdateConfig(config shardmeta.ClusterConfig) (shardmeta.ClusterConfig, error) {
	if err := config.Validate(); err != nil {
		return shardmeta.ClusterConfig{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		if err := s.store.Save(context.Background(), config); err != nil {
			return shardmeta.ClusterConfig{}, err
		}
	}
	s.config = config
	return s.config, nil
}

func (s *Service) LockShards(shardIDs ...shardmeta.ShardID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, shardID := range uniqueShardIDs(shardIDs) {
		if _, ok := s.lockedShardIDs[shardID]; ok {
			return ShardMigrationError{ShardID: shardID}
		}
	}
	for _, shardID := range uniqueShardIDs(shardIDs) {
		s.lockedShardIDs[shardID] = struct{}{}
	}
	return nil
}

func (s *Service) UnlockShards(shardIDs ...shardmeta.ShardID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, shardID := range uniqueShardIDs(shardIDs) {
		delete(s.lockedShardIDs, shardID)
	}
}

func (s *Service) IsShardLocked(shardID shardmeta.ShardID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.lockedShardIDs[shardID]
	return ok
}

func (s *Service) HasLockedShards() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.lockedShardIDs) > 0
}

func (s *Service) WithLockedShards(shardIDs []shardmeta.ShardID, fn func() error) error {
	if err := s.LockShards(shardIDs...); err != nil {
		return err
	}
	defer s.UnlockShards(shardIDs...)
	return fn()
}

func (s *Service) refreshLocked(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	config, err := s.store.Load(ctx)
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return nil
		}
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	s.config = config
	return nil
}

func uniqueShardIDs(shardIDs []shardmeta.ShardID) []shardmeta.ShardID {
	if len(shardIDs) < 2 {
		return shardIDs
	}
	seen := make(map[shardmeta.ShardID]struct{}, len(shardIDs))
	unique := make([]shardmeta.ShardID, 0, len(shardIDs))
	for _, shardID := range shardIDs {
		if _, ok := seen[shardID]; ok {
			continue
		}
		seen[shardID] = struct{}{}
		unique = append(unique, shardID)
	}
	return unique
}
