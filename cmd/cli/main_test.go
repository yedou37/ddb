package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/model"
)

func TestExecuteSQL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sql" {
			t.Fatalf("r.URL.Path = %q, want /sql", r.URL.Path)
		}
		writeJSON(t, w, model.SQLResponse{
			Success: true,
			Result:  model.QueryResult{Type: "select", Rows: [][]any{{1, "alice"}}},
		})
	}))
	defer server.Close()

	response, err := executeSQL(server.URL, "SELECT * FROM t")
	if err != nil {
		t.Fatalf("executeSQL() error = %v", err)
	}
	if !response.Success {
		t.Fatalf("response.Success = false, want true")
	}
}

func TestGetJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, model.NodeInfo{ID: "node1", HTTPAddr: "127.0.0.1:8080", IsLeader: true})
	}))
	defer server.Close()

	info, err := getJSON[model.NodeInfo](server.URL)
	if err != nil {
		t.Fatalf("getJSON() error = %v", err)
	}
	if !info.IsLeader || info.ID != "node1" {
		t.Fatalf("info = %#v, want leader node1", info)
	}
}

func TestReadURLAndNormalizeURL(t *testing.T) {
	url, err := readURL(context.Background(), config.CLIConfig{NodeURL: ":8080"}, nil)
	if err != nil {
		t.Fatalf("readURL() error = %v", err)
	}
	if got, want := url, "http://127.0.0.1:8080"; got != want {
		t.Fatalf("readURL() = %q, want %q", got, want)
	}

	if got, want := normalizeURL("127.0.0.1:8081"), "http://127.0.0.1:8081"; got != want {
		t.Fatalf("normalizeURL() = %q, want %q", got, want)
	}
}

func TestIsWrite(t *testing.T) {
	if !isWrite(model.StatementInsert) {
		t.Fatalf("isWrite(insert) = false, want true")
	}
	if isWrite(model.StatementSelect) {
		t.Fatalf("isWrite(select) = true, want false")
	}
}

func TestPrintJSON(t *testing.T) {
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = oldStdout
	}()

	if err := printJSON(map[string]string{"status": "ok"}); err != nil {
		t.Fatalf("printJSON() error = %v", err)
	}
	_ = writer.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	if !strings.Contains(string(data), "\"status\": \"ok\"") {
		t.Fatalf("printJSON() output = %q, want status", string(data))
	}
}

func TestRunClusterMembers(t *testing.T) {
	var baseURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/members":
			writeJSON(t, w, []model.ClusterMember{{ID: "node1", Online: true, InRaft: true, Status: "online-voter"}})
		case "/leader":
			writeJSON(t, w, model.NodeInfo{ID: "node1", HTTPAddr: baseURL, IsLeader: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	baseURL = server.URL

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = oldStdout
	}()

	err = runCluster(context.Background(), config.CLIConfig{NodeURL: server.URL}, nil, []string{"members"})
	_ = writer.Close()
	if err != nil {
		t.Fatalf("runCluster() error = %v", err)
	}

	data, _ := io.ReadAll(reader)
	if !strings.Contains(string(data), "\"id\": \"node1\"") {
		t.Fatalf("runCluster() output = %q, want node1", string(data))
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("json.Encode() error = %v", err)
	}
}

func TestRemoveNodeAndRejoinNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("r.Method = %q, want POST", r.Method)
		}
		switch r.URL.Path {
		case "/remove":
			writeJSON(t, w, map[string]string{"status": "removed"})
		case "/rejoin":
			buf := new(bytes.Buffer)
			_, _ = io.Copy(buf, r.Body)
			if !strings.Contains(buf.String(), "node3") {
				t.Fatalf("rejoin body = %q, want node3", buf.String())
			}
			writeJSON(t, w, map[string]string{"status": "rejoined"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = oldStdout
	}()

	if err := removeNode(server.URL, "node3"); err != nil {
		t.Fatalf("removeNode() error = %v", err)
	}
	if err := rejoinNode(server.URL, "node3", "127.0.0.1:7003", "127.0.0.1:8003"); err != nil {
		t.Fatalf("rejoinNode() error = %v", err)
	}
	_ = writer.Close()
	_, _ = io.ReadAll(reader)
}
