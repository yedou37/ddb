package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yedou37/dbd/internal/model"
	"github.com/yedou37/dbd/internal/service"
	"github.com/yedou37/dbd/internal/storage"
)

func TestHandlerHealthAndSQL(t *testing.T) {
	queryService := newStandaloneQueryService(t)
	handler := NewHandler(queryService)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("/health response = %d %q", rec.Code, rec.Body.String())
	}

	body, _ := json.Marshal(model.SQLRequest{SQL: "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)"})
	req = httptest.NewRequest(http.MethodPost, "/sql", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "create_table") {
		t.Fatalf("/sql create response = %d %q", rec.Code, rec.Body.String())
	}
}

func TestHandlerStatusMembersAndTables(t *testing.T) {
	queryService := newStandaloneQueryService(t)
	_, _ = queryService.Execute(context.Background(), "CREATE TABLE books (id INT PRIMARY KEY, name TEXT)")
	handler := NewHandler(queryService)

	for _, path := range []string{"/status", "/members", "/tables", "/leader"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s response code = %d, want 200", path, rec.Code)
		}
	}
}

func TestHandlerRemoveWithoutRaft(t *testing.T) {
	queryService := newStandaloneQueryService(t)
	handler := NewHandler(queryService)

	body, _ := json.Marshal(model.RemoveRequest{NodeID: "node2"})
	req := httptest.NewRequest(http.MethodPost, "/remove", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("/remove response code = %d, want 400", rec.Code)
	}
}

func newStandaloneQueryService(t *testing.T) *service.QueryService {
	t.Helper()

	store, err := storage.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	return service.NewQueryService("node1", "127.0.0.1:8080", "127.0.0.1:7000", store, nil, nil)
}
