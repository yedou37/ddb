package main

import (
	"bufio"
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

const usageText = "usage: cli [--etcd=host:2379] [--node-url=http://host:8080] sql \"SELECT * FROM users\" | inspect \"SELECT * FROM users\" | cluster status|leader|members|tables|remove|rejoin | control config|groups|shards|move-shard|rebalance | interact"

func main() {
	cfg, args := config.ParseCLIConfig()
	if len(args) == 0 {
		log.Fatal(usageText)
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

	if err := dispatchCommand(context.Background(), cfg, discoveryClient, args); err != nil {
		log.Fatal(err)
	}
}

func dispatchCommand(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf(usageText)
	}

	switch args[0] {
	case "sql":
		if len(args) < 2 {
			return fmt.Errorf("sql command requires a statement")
		}
		return runSQL(ctx, cfg, discoveryClient, args[1])
	case "inspect":
		if len(args) < 2 {
			return fmt.Errorf("inspect command requires a statement")
		}
		return runInspect(cfg, args[1])
	case "cluster":
		if len(args) < 2 {
			return fmt.Errorf("cluster command requires a subcommand")
		}
		return runCluster(ctx, cfg, discoveryClient, args[1:])
	case "control":
		if len(args) < 2 {
			return fmt.Errorf("control command requires a subcommand")
		}
		return runControl(ctx, cfg, discoveryClient, args[1:])
	case "interact":
		return runInteractive(ctx, cfg, discoveryClient)
	default:
		return fmt.Errorf("unknown command %s", args[0])
	}
}

func runInteractive(ctx context.Context, cfg config.CLIConfig, discoveryClient *discovery.Client) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Fprintln(os.Stdout, "ddb interactive mode")
	if cfg.NodeURL != "" {
		fmt.Fprintf(os.Stdout, "target: %s\n", normalizeURL(cfg.NodeURL))
	} else if len(cfg.ETCDEndpoints) > 0 {
		fmt.Fprintf(os.Stdout, "discovery: %s\n", strings.Join(cfg.ETCDEndpoints, ","))
	} else {
		fmt.Fprintln(os.Stdout, "warning: set --node-url or --etcd before running interact")
	}
	fmt.Fprintln(os.Stdout, "enter commands like `sql SELECT * FROM users WHERE id = 1`")
	fmt.Fprintln(os.Stdout, "type `help` for usage, `exit` or `quit` to leave")

	for {
		fmt.Fprint(os.Stdout, "ddb> ")
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			if err == io.EOF {
				fmt.Fprintln(os.Stdout)
				return nil
			}
			continue
		}

		switch strings.ToLower(line) {
		case "exit", "quit":
			return nil
		case "help":
			fmt.Fprintln(os.Stdout, usageText)
			fmt.Fprintln(os.Stdout, "interactive examples:")
			fmt.Fprintln(os.Stdout, "  sql SELECT * FROM users WHERE id = 1")
			fmt.Fprintln(os.Stdout, "  inspect SELECT * FROM users WHERE id = 1")
			fmt.Fprintln(os.Stdout, "  control groups")
			fmt.Fprintln(os.Stdout, "  control move-shard 6 g3")
			fmt.Fprintln(os.Stdout, "  cluster status")
		default:
			commandArgs, parseErr := parseInteractiveCommand(line)
			if parseErr != nil {
				fmt.Fprintf(os.Stdout, "error: %v\n", parseErr)
			} else if runErr := dispatchCommand(ctx, cfg, discoveryClient, commandArgs); runErr != nil {
				fmt.Fprintf(os.Stdout, "error: %v\n", runErr)
			}
		}

		if err == io.EOF {
			fmt.Fprintln(os.Stdout)
			return nil
		}
	}
}

func parseInteractiveCommand(line string) ([]string, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil, fmt.Errorf("empty command")
	}

	if sqlText, ok := strings.CutPrefix(trimmed, "sql "); ok {
		statement := trimOptionalQuotes(strings.TrimSpace(sqlText))
		if statement == "" {
			return nil, fmt.Errorf("sql command requires a statement")
		}
		return []string{"sql", statement}, nil
	}
	if trimmed == "sql" {
		return nil, fmt.Errorf("sql command requires a statement")
	}
	if inspectText, ok := strings.CutPrefix(trimmed, "inspect "); ok {
		statement := trimOptionalQuotes(strings.TrimSpace(inspectText))
		if statement == "" {
			return nil, fmt.Errorf("inspect command requires a statement")
		}
		return []string{"inspect", statement}, nil
	}
	if trimmed == "inspect" {
		return nil, fmt.Errorf("inspect command requires a statement")
	}

	args := strings.Fields(trimmed)
	if len(args) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	if args[0] == "interact" {
		return nil, fmt.Errorf("already in interactive mode")
	}
	return args, nil
}

func trimOptionalQuotes(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
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

func runInspect(cfg config.CLIConfig, statement string) error {
	if strings.TrimSpace(cfg.NodeURL) == "" {
		return fmt.Errorf("inspect requires --node-url pointing to a specific node")
	}

	parsed, err := sqlparser.Parse(statement)
	if err != nil {
		return err
	}
	if parsed.Type != model.StatementSelect && parsed.Type != model.StatementShowTables {
		return fmt.Errorf("inspect only supports read-only statements")
	}

	response, err := executeSQL(cfg.NodeURL, statement)
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
