package config

import (
	"flag"
	"os"
	"strings"
)

type ServerConfig struct {
	NodeID        string
	HTTPAddr      string
	RaftAddr      string
	RaftDir       string
	Bootstrap     bool
	Rejoin        bool
	JoinAddr      string
	DBPath        string
	ETCDEndpoints []string
}

type CLIConfig struct {
	NodeURL       string
	ETCDEndpoints []string
}

func ParseServerConfig() ServerConfig {
	nodeID := flag.String("node-id", envOrDefault("NODE_ID", "node1"), "")
	httpAddr := flag.String("http-addr", envOrDefault("HTTP_ADDR", ":8080"), "")
	raftAddr := flag.String("raft-addr", envOrDefault("RAFT_ADDR", ":7000"), "")
	raftDir := flag.String("raft-dir", envOrDefault("RAFT_DIR", "raft"), "")
	bootstrap := flag.Bool("bootstrap", envOrDefault("BOOTSTRAP", "false") == "true", "")
	rejoin := flag.Bool("rejoin", envOrDefault("REJOIN", "false") == "true", "")
	joinAddr := flag.String("join", envOrDefault("JOIN_ADDR", ""), "")
	dbPath := flag.String("db-path", envOrDefault("DB_PATH", "data.db"), "")
	etcd := flag.String("etcd", envOrDefault("ETCD_ADDR", ""), "")
	flag.Parse()

	// Rejoin is a recovery path and must not bootstrap a brand-new cluster.
	if *rejoin {
		*bootstrap = false
	}

	return ServerConfig{
		NodeID:        *nodeID,
		HTTPAddr:      *httpAddr,
		RaftAddr:      *raftAddr,
		RaftDir:       *raftDir,
		Bootstrap:     *bootstrap,
		Rejoin:        *rejoin,
		JoinAddr:      *joinAddr,
		DBPath:        *dbPath,
		ETCDEndpoints: splitCSV(*etcd),
	}
}

func ParseCLIConfig() (CLIConfig, []string) {
	nodeURL := flag.String("node-url", envOrDefault("NODE_URL", ""), "")
	etcd := flag.String("etcd", envOrDefault("ETCD_ADDR", ""), "")
	flag.Parse()

	return CLIConfig{
		NodeURL:       *nodeURL,
		ETCDEndpoints: splitCSV(*etcd),
	}, flag.Args()
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
