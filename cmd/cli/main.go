package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yedou37/dbd/internal/config"
	"github.com/yedou37/dbd/internal/discovery"
	"github.com/yedou37/dbd/internal/model"
	sqlparser "github.com/yedou37/dbd/internal/sql"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

func main() {
	cfg, args := config.ParseCLIConfig()
	if len(args) == 0 {
		log.Fatal("usage: cli [--etcd=host:2379] [--node-url=http://host:8080] sql \"SELECT * FROM users\" | cluster status|leader|members|tables")
	}

	discoveryClient, err := discovery.New(cfg.ETCDEndpoints)
	if err != nil {
		log.Fatal(err)
	}
	if discoveryClient != nil {
		defer func() {
			_ = discoveryClient.Close()
		}()
	}

	switch args[0] {
	case "sql":
		if len(args) < 2 {
			log.Fatal("sql command requires a statement")
		}
		if err := runSQL(context.Background(), cfg, discoveryClient, args[1]); err != nil {
			log.Fatal(err)
		}
	case "cluster":
		if len(args) < 2 {
			log.Fatal("cluster command requires a subcommand")
		}
		if err := runCluster(context.Background(), cfg, discoveryClient, args[1]); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown command %s", args[0])
	}
}

func runSQL(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client, statement string) error {
	parsed, err := sqlparser.Parse(statement)
	if err != nil {
		return err
	}

	targetURL := cfg.NodeURL
	if isWrite(parsed.Type) {
		targetURL, err = leaderURL(ctx, cfg, discoveryClient)
		if err != nil {
			return err
		}
	} else if targetURL == "" {
		targetURL, err = leaderURL(ctx, cfg, discoveryClient)
		if err != nil {
			return err
		}
	}

	response, err := executeSQL(targetURL, statement)
	if err != nil {
		return err
	}

	return printJSON(response)
}

func runCluster(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client, command string) error {
	switch command {
	case "leader":
		url, err := leaderURL(ctx, cfg, discoveryClient)
		if err != nil {
			return err
		}
		status, err := getJSON[model.NodeInfo](url + "/leader")
		if err != nil {
			return err
		}
		return printJSON(status)
	case "members":
		members, err := members(ctx, cfg, discoveryClient)
		if err != nil {
			return err
		}
		return printJSON(members)
	case "status":
		members, err := members(ctx, cfg, discoveryClient)
		if err != nil {
			return err
		}
		statuses := make([]model.StatusResponse, 0, len(members))
		for _, member := range members {
			status, err := getJSON[model.StatusResponse](normalizeURL(member.HTTPAddr) + "/status")
			if err != nil {
				return err
			}
			statuses = append(statuses, status)
		}
		return printJSON(statuses)
	case "tables":
		url, err := leaderURL(ctx, cfg, discoveryClient)
		if err != nil {
			return err
		}
		result, err := getJSON[model.SQLResponse](url + "/tables")
		if err != nil {
			return err
		}
		return printJSON(result)
	default:
		return fmt.Errorf("unknown cluster command %s", command)
	}
}

func executeSQL(baseURL, statement string) (model.SQLResponse, error) {
	body, err := json.Marshal(model.SQLRequest{SQL: statement})
	if err != nil {
		return model.SQLResponse{}, err
	}

	response, err := httpClient.Post(normalizeURL(baseURL)+"/sql", "application/json", bytes.NewReader(body))
	if err != nil {
		return model.SQLResponse{}, err
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return model.SQLResponse{}, err
	}

	var parsed model.SQLResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return model.SQLResponse{}, err
	}
	if response.StatusCode >= 400 {
		return parsed, fmt.Errorf("%s", parsed.Error)
	}
	return parsed, nil
}

func getJSON[T any](url string) (T, error) {
	var result T
	response, err := httpClient.Get(normalizeURL(url))
	if err != nil {
		return result, err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(response.Body)
		return result, fmt.Errorf("%s", string(body))
	}

	err = json.NewDecoder(response.Body).Decode(&result)
	return result, err
}

func leaderURL(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client) (string, error) {
	if cfg.NodeURL != "" {
		leader, err := getJSON[model.NodeInfo](normalizeURL(cfg.NodeURL) + "/leader")
		if err == nil && leader.HTTPAddr != "" {
			return normalizeURL(leader.HTTPAddr), nil
		}
	}

	if discoveryClient != nil {
		nodes, err := discoveryClient.ListNodes(ctx)
		if err == nil && len(nodes) > 0 {
			for _, node := range nodes {
				if node.HTTPAddr == "" {
					continue
				}
				leader, err := getJSON[model.NodeInfo](normalizeURL(node.HTTPAddr) + "/leader")
				if err == nil && leader.HTTPAddr != "" {
					return normalizeURL(leader.HTTPAddr), nil
				}
			}
			for _, node := range nodes {
				if node.IsLeader && node.HTTPAddr != "" {
					return normalizeURL(node.HTTPAddr), nil
				}
			}
		}
	}

	return "", fmt.Errorf("leader not found: set --etcd or --node-url")
}

func members(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client) ([]model.NodeInfo, error) {
	if discoveryClient != nil {
		list, err := discoveryClient.ListNodes(ctx)
		if err == nil && len(list) > 0 {
			return list, nil
		}
	}

	if cfg.NodeURL == "" {
		return nil, fmt.Errorf("members not found: set --etcd or --node-url")
	}

	list, err := getJSON[[]model.NodeInfo](normalizeURL(cfg.NodeURL) + "/members")
	if err != nil {
		return nil, err
	}
	return list, nil
}

func isWrite(statementType model.StatementType) bool {
	return statementType == model.StatementCreateTable || statementType == model.StatementInsert || statementType == model.StatementDelete
}

func normalizeURL(value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if strings.HasPrefix(value, ":") {
		return "http://127.0.0.1" + value
	}
	return "http://" + value
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
