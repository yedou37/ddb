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
	"strconv"
	"strings"
	"time"

	"github.com/yedou37/ddb/internal/config"
	"github.com/yedou37/ddb/internal/discovery"
	"github.com/yedou37/ddb/internal/model"
	sqlparser "github.com/yedou37/ddb/internal/sql"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

func main() {
	cfg, args := config.ParseCLIConfig()
	if len(args) == 0 {
		log.Fatal("usage: cli [--etcd=host:2379] [--node-url=http://host:8080] sql \"SELECT * FROM users\" | cluster status|leader|members|tables | control config|groups|shards|move-shard|rebalance")
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
		if err := runCluster(context.Background(), cfg, discoveryClient, args[1:]); err != nil {
			log.Fatal(err)
		}
	case "control":
		if len(args) < 2 {
			log.Fatal("control command requires a subcommand")
		}
		if err := runControl(context.Background(), cfg, discoveryClient, args[1:]); err != nil {
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

	targetURL, controlErr := controlURL(ctx, cfg, discoveryClient)
	if controlErr == nil {
		response, execErr := executeSQL(targetURL, statement)
		if execErr != nil {
			return execErr
		}
		return printJSON(response)
	}

	targetURL = cfg.NodeURL
	if isWrite(parsed.Type) {
		targetURL, err = leaderURL(ctx, cfg, discoveryClient)
		if err != nil {
			return err
		}
	} else if targetURL == "" {
		targetURL, err = readURL(ctx, cfg, discoveryClient)
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

func runControl(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client, args []string) error {
	baseURL, err := controlURL(ctx, cfg, discoveryClient)
	if err != nil {
		return err
	}

	switch args[0] {
	case "config":
		result, err := getJSON[any](baseURL + "/config")
		if err != nil {
			return err
		}
		return printJSON(result)
	case "groups":
		result, err := getJSON[[]model.GroupStatus](baseURL + "/groups")
		if err != nil {
			return err
		}
		return printJSON(result)
	case "shards":
		result, err := getJSON[model.ShardsResponse](baseURL + "/shards")
		if err != nil {
			return err
		}
		return printJSON(result)
	case "move-shard":
		if len(args) < 3 {
			return fmt.Errorf("control move-shard requires: <shard-id> <group-id>")
		}
		shardID, err := parseShardID(args[1])
		if err != nil {
			return err
		}
		result, err := postJSON[model.ShardsResponse](baseURL+"/move-shard", map[string]any{
			"shard_id": shardID,
			"group_id": args[2],
		})
		if err != nil {
			return err
		}
		return printJSON(result)
	case "rebalance":
		if len(args) < 2 {
			return fmt.Errorf("control rebalance requires at least one group id")
		}
		groupIDs := make([]string, 0, len(args)-1)
		for _, value := range args[1:] {
			groupIDs = append(groupIDs, value)
		}
		result, err := postJSON[model.ShardsResponse](baseURL+"/rebalance", map[string]any{
			"group_ids": groupIDs,
		})
		if err != nil {
			return err
		}
		return printJSON(result)
	default:
		return fmt.Errorf("unknown control command %s", args[0])
	}
}

func runCluster(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client, args []string) error {
	command := args[0]
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
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("cluster remove requires a node id")
		}
		target := args[1]
		leader, err := leaderURL(ctx, cfg, discoveryClient)
		if err != nil {
			return err
		}
		return removeNode(leader, target)
	case "rejoin":
		if len(args) < 4 {
			return fmt.Errorf("cluster rejoin requires: <node-id> <raft-addr> <http-addr>")
		}
		leader, err := leaderURL(ctx, cfg, discoveryClient)
		if err != nil {
			return err
		}
		return rejoinNode(leader, args[1], args[2], args[3])
	case "status":
		members, err := members(ctx, cfg, discoveryClient)
		if err != nil {
			return err
		}
		statuses := make([]model.StatusResponse, 0, len(members))
		for _, member := range members {
			if member.Removed || !member.Online || member.HTTPAddr == "" {
				continue
			}
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

func postJSON[T any](url string, payload any) (T, error) {
	var result T
	body, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}

	response, err := httpClient.Post(normalizeURL(url), "application/json", bytes.NewReader(body))
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
	if discoveryClient != nil {
		leader, err := discoveryClient.FindLeader(ctx)
		if err == nil && leader.HTTPAddr != "" {
			return normalizeURL(leader.HTTPAddr), nil
		}
	}

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

func controlURL(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client) (string, error) {
	if cfg.NodeURL != "" {
		return normalizeURL(cfg.NodeURL), nil
	}
	if discoveryClient != nil {
		nodes, err := discoveryClient.ListNodes(ctx)
		if err == nil {
			for _, node := range nodes {
				if node.HTTPAddr == "" {
					continue
				}
				if node.Role == modelRoleAPIServer() || node.Role == modelRoleController() {
					return normalizeURL(node.HTTPAddr), nil
				}
			}
		}
	}
	return "", fmt.Errorf("control target not found: set --etcd or --node-url")
}

func readURL(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client) (string, error) {
	if cfg.NodeURL != "" {
		return normalizeURL(cfg.NodeURL), nil
	}

	if discoveryClient != nil {
		nodes, err := discoveryClient.ListNodes(ctx)
		if err == nil {
			for _, node := range nodes {
				if node.HTTPAddr != "" {
					return normalizeURL(node.HTTPAddr), nil
				}
			}
		}
	}

	return "", fmt.Errorf("read target not found: set --etcd or --node-url")
}

func members(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client) ([]model.ClusterMember, error) {
	if discoveryClient != nil {
		leader, err := leaderURL(ctx, cfg, discoveryClient)
		if err == nil {
			list, err := getJSON[[]model.ClusterMember](normalizeURL(leader) + "/members")
			if err == nil && len(list) > 0 {
				return list, nil
			}
		}
	}

	if cfg.NodeURL == "" {
		return nil, fmt.Errorf("members not found: set --etcd or --node-url")
	}

	list, err := getJSON[[]model.ClusterMember](normalizeURL(cfg.NodeURL) + "/members")
	if err != nil {
		return nil, err
	}
	return list, nil
}

func removeNode(baseURL, nodeID string) error {
	body, err := json.Marshal(model.RemoveRequest{NodeID: nodeID})
	if err != nil {
		return err
	}

	response, err := httpClient.Post(normalizeURL(baseURL)+"/remove", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		payload, _ := io.ReadAll(response.Body)
		return fmt.Errorf("%s", string(payload))
	}

	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return err
	}
	return printJSON(payload)
}

func rejoinNode(baseURL, nodeID, raftAddr, httpAddr string) error {
	body, err := json.Marshal(model.JoinRequest{
		NodeID:   nodeID,
		RaftAddr: raftAddr,
		HTTPAddr: httpAddr,
	})
	if err != nil {
		return err
	}

	response, err := httpClient.Post(normalizeURL(baseURL)+"/rejoin", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		payload, _ := io.ReadAll(response.Body)
		return fmt.Errorf("%s", string(payload))
	}

	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return err
	}
	return printJSON(payload)
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

func parseShardID(value string) (uint32, error) {
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid shard id %q: %w", value, err)
	}
	return uint32(parsed), nil
}

func modelRoleAPIServer() string {
	return "apiserver"
}

func modelRoleController() string {
	return "controller"
}
