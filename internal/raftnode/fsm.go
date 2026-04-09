package raftnode

import (
	"io"

	"github.com/hashicorp/raft"

	"github.com/yedou37/dbd/internal/model"
	sqlparser "github.com/yedou37/dbd/internal/sql"
	"github.com/yedou37/dbd/internal/storage"
)

type fsm struct {
	store *storage.Store
}

func newFSM(store *storage.Store) *fsm {
	return &fsm{store: store}
}

func (f *fsm) Apply(log *raft.Log) any {
	statement, err := sqlparser.Parse(string(log.Data))
	if err != nil {
		return err
	}
	result, err := f.store.ExecuteStatement(statement)
	if err != nil {
		return err
	}
	return result
}

func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	return snapshot{}, nil
}

func (f *fsm) Restore(io.ReadCloser) error {
	return nil
}

type snapshot struct{}

func (snapshot) Persist(raft.SnapshotSink) error {
	return nil
}

func (snapshot) Release() {}

func decodeApplyResponse(response any) (model.QueryResult, error) {
	switch typed := response.(type) {
	case model.QueryResult:
		return typed, nil
	case error:
		return model.QueryResult{}, typed
	default:
		return model.QueryResult{}, nil
	}
}
