package config

import (
	"flag"
	"os"
	"testing"
)

func TestParseServerConfigFromEnv(t *testing.T) {
	resetFlags(t)
	t.Setenv("NODE_ID", "node9")
	t.Setenv("HTTP_ADDR", "127.0.0.1:9999")
	t.Setenv("RAFT_ADDR", "127.0.0.1:7999")
	t.Setenv("RAFT_DIR", "/tmp/raft-dir")
	t.Setenv("BOOTSTRAP", "true")
	t.Setenv("REJOIN", "true")
	t.Setenv("JOIN_ADDR", "127.0.0.1:8080")
	t.Setenv("DB_PATH", "/tmp/data.db")
	t.Setenv("ETCD_ADDR", "127.0.0.1:2379,127.0.0.1:2380")

	cfg := ParseServerConfig()

	if cfg.NodeID != "node9" || cfg.HTTPAddr != "127.0.0.1:9999" || cfg.RaftAddr != "127.0.0.1:7999" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if !cfg.Rejoin {
		t.Fatalf("cfg.Rejoin = false, want true")
	}
	if cfg.Bootstrap {
		t.Fatalf("cfg.Bootstrap = true, want false when rejoin=true")
	}
	if got, want := len(cfg.ETCDEndpoints), 2; got != want {
		t.Fatalf("len(cfg.ETCDEndpoints) = %d, want %d", got, want)
	}
}

func TestParseCLIConfigFromArgs(t *testing.T) {
	resetFlags(t)
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()
	os.Args = []string{"cli", "--node-url=http://127.0.0.1:8080", "--etcd=127.0.0.1:2379", "cluster", "members"}

	cfg, args := ParseCLIConfig()

	if cfg.NodeURL != "http://127.0.0.1:8080" {
		t.Fatalf("cfg.NodeURL = %q, want http://127.0.0.1:8080", cfg.NodeURL)
	}
	if got, want := len(cfg.ETCDEndpoints), 1; got != want {
		t.Fatalf("len(cfg.ETCDEndpoints) = %d, want %d", got, want)
	}
	if got, want := len(args), 2; got != want {
		t.Fatalf("len(args) = %d, want %d", got, want)
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" a , , b , c ")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("splitCSV() = %#v, want [a b c]", got)
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("CUSTOM_ENV", "value")
	if got := envOrDefault("CUSTOM_ENV", "fallback"); got != "value" {
		t.Fatalf("envOrDefault() = %q, want value", got)
	}
	if got := envOrDefault("UNKNOWN_ENV", "fallback"); got != "fallback" {
		t.Fatalf("envOrDefault() = %q, want fallback", got)
	}
}

func resetFlags(t *testing.T) {
	t.Helper()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
}
