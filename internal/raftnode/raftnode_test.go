package raftnode

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hashicorp/raft"

	"github.com/yedou37/dbd/internal/model"
	"github.com/yedou37/dbd/internal/storage"
)

func TestDecodeApplyResponse(t *testing.T) {
	result, err := decodeApplyResponse(model.QueryResult{Type: "insert", RowsAffected: 1})
	if err != nil {
		t.Fatalf("decodeApplyResponse() error = %v", err)
	}
	if result.Type != "insert" {
		t.Fatalf("result.Type = %q, want insert", result.Type)
	}

	_, err = decodeApplyResponse(errors.New("boom"))
	if err == nil {
		t.Fatalf("decodeApplyResponse(error) = nil, want error")
	}
}

func TestNormalizeHTTPAddrAndRaftStateDir(t *testing.T) {
	if got, want := normalizeHTTPAddr("127.0.0.1:8080"), "http://127.0.0.1:8080"; got != want {
		t.Fatalf("normalizeHTTPAddr() = %q, want %q", got, want)
	}
	if got, want := raftStateDir("/tmp/raft", "node1"), filepath.Join("/tmp/raft", "node1"); got != want {
		t.Fatalf("raftStateDir() = %q, want %q", got, want)
	}
}

func TestFSMApply(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "fsm.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	fsm := newFSM(store)
	createResult := fsm.Apply(&raft.Log{Data: []byte("CREATE TABLE users (id INT PRIMARY KEY, name TEXT)")})
	if _, err := decodeApplyResponse(createResult); err != nil {
		t.Fatalf("fsm.Apply(create) error = %v", err)
	}

	insertResult := fsm.Apply(&raft.Log{Data: []byte("INSERT INTO users VALUES (1, 'alice')")})
	if _, err := decodeApplyResponse(insertResult); err != nil {
		t.Fatalf("fsm.Apply(insert) error = %v", err)
	}
}
