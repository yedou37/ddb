package coordinator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yedou37/ddb/internal/model"
	"github.com/yedou37/ddb/internal/router"
	"github.com/yedou37/ddb/internal/shardmeta"
	sqlparser "github.com/yedou37/ddb/internal/sql"
)

var (
	ErrCoordinatorUnavailable = errors.New("coordinator is not configured")
	ErrNoShardNodesAvailable  = errors.New("no shard nodes available for group")
	ErrRouteKeyRequired       = errors.New("single-shard routing requires a primary key value")
	ErrShardMigrationBlocked  = errors.New("shard migration is in progress")
	ErrJoinWhereUnsupported   = errors.New("JOIN with WHERE is not supported")
)

type ShardMigrationError struct {
	ShardID shardmeta.ShardID
}

func (e ShardMigrationError) Error() string {
	return fmt.Sprintf("shard %d is migrating, retry later", e.ShardID)
}

func (e ShardMigrationError) Unwrap() error {
	return ErrShardMigrationBlocked
}

type ConfigReader interface {
	CurrentConfig() shardmeta.ClusterConfig
}

type NodeLister interface {
	ListNodes(ctx context.Context) ([]model.NodeInfo, error)
}

type ShardLockChecker interface {
	IsShardLocked(shardID shardmeta.ShardID) bool
	HasLockedShards() bool
}

type Coordinator struct {
	configReader ConfigReader
	nodeLister   NodeLister
	router       *router.Router
	httpClient   *http.Client
}

func New(configReader ConfigReader, nodeLister NodeLister, routeEngine *router.Router) *Coordinator {
	return &Coordinator{
		configReader: configReader,
		nodeLister:   nodeLister,
		router:       routeEngine,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Coordinator) ExecuteSQL(ctx context.Context, input string) (model.SQLResponse, error) {
	if c == nil || c.configReader == nil || c.nodeLister == nil || c.router == nil {
		return model.SQLResponse{}, ErrCoordinatorUnavailable
	}

	statement, err := sqlparser.Parse(input)
	if err != nil {
		return model.SQLResponse{}, err
	}

	switch statement.Type {
	case model.StatementCreateTable:
		return c.broadcastSQL(ctx, input)
	case model.StatementInsert:
		if len(statement.Values) == 0 {
			return model.SQLResponse{}, ErrRouteKeyRequired
		}
		return c.routeAndExecute(ctx, statement.Table, statement.Values[0], input)
	case model.StatementSelect:
		return c.executeSelect(ctx, statement, input)
	case model.StatementDelete:
		if statement.Filter == nil {
			return model.SQLResponse{}, ErrRouteKeyRequired
		}
		return c.routeAndExecute(ctx, statement.Table, statement.Filter.Value, input)
	case model.StatementShowTables:
		return c.broadcastShowTables(ctx, input)
	default:
		return model.SQLResponse{}, fmt.Errorf("unsupported statement type %s", statement.Type)
	}
}

func (c *Coordinator) executeSelect(ctx context.Context, statement model.Statement, input string) (model.SQLResponse, error) {
	if statement.Join != nil {
		return c.executeJoin(ctx, statement)
	}
	if statement.Filter == nil {
		return c.scatterSelect(ctx, input)
	}

	schema, err := c.schemaForTable(ctx, statement.Table)
	if err != nil {
		return model.SQLResponse{}, err
	}
	if strings.EqualFold(statement.Filter.Column, schema.PrimaryKey) {
		return c.routeAndExecute(ctx, statement.Table, statement.Filter.Value, input)
	}
	return c.scatterSelect(ctx, input)
}

func (c *Coordinator) MigrateShard(ctx context.Context, shardID shardmeta.ShardID, sourceGroup, targetGroup shardmeta.GroupID) error {
	if c == nil || c.configReader == nil || c.nodeLister == nil || c.router == nil {
		return ErrCoordinatorUnavailable
	}
	if sourceGroup == "" || targetGroup == "" || sourceGroup == targetGroup {
		return nil
	}

	sourceNode, err := c.pickGroupNode(ctx, sourceGroup)
	if err != nil {
		return err
	}
	targetNode, err := c.pickGroupNode(ctx, targetGroup)
	if err != nil {
		return err
	}
	tablesResponse, err := c.executeRemoteSQL(ctx, sourceNode.HTTPAddr, "SHOW TABLES")
	if err != nil {
		return err
	}

	for _, table := range tablesResponse.Result.Tables {
		schema, err := c.fetchSchema(ctx, sourceNode.HTTPAddr, table)
		if err != nil {
			return err
		}
		if ensureErr := c.ensureTable(ctx, targetNode.HTTPAddr, schema); ensureErr != nil {
			return ensureErr
		}

		selectResponse, err := c.executeRemoteSQL(ctx, sourceNode.HTTPAddr, "SELECT * FROM "+table)
		if err != nil {
			return err
		}
		pkIndex := primaryKeyIndex(schema)
		if pkIndex < 0 {
			return fmt.Errorf("primary key not found for table %s", table)
		}

		for _, row := range selectResponse.Result.Rows {
			if pkIndex >= len(row) {
				return fmt.Errorf("row shape does not match schema for table %s", table)
			}
			routeResult, err := c.router.Route(table, row[pkIndex], c.configReader.CurrentConfig())
			if err != nil {
				return err
			}
			if routeResult.ShardID != shardID {
				continue
			}
			if err := c.insertRow(ctx, targetNode.HTTPAddr, table, row); err != nil {
				return err
			}
			if err := c.deleteRow(ctx, sourceNode.HTTPAddr, table, schema.PrimaryKey, row[pkIndex]); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Coordinator) routeAndExecute(ctx context.Context, table string, primaryKey any, input string) (model.SQLResponse, error) {
	config := c.configReader.CurrentConfig()
	result, err := c.router.Route(table, primaryKey, config)
	if err != nil {
		return model.SQLResponse{}, err
	}
	if checker, ok := c.configReader.(ShardLockChecker); ok && checker.IsShardLocked(result.ShardID) {
		return model.SQLResponse{}, ShardMigrationError{ShardID: result.ShardID}
	}
	node, err := c.pickGroupNode(ctx, result.GroupID)
	if err != nil {
		return model.SQLResponse{}, err
	}
	return c.executeRemoteSQL(ctx, node.HTTPAddr, input)
}

func (c *Coordinator) scatterSelect(ctx context.Context, input string) (model.SQLResponse, error) {
	if checker, ok := c.configReader.(ShardLockChecker); ok && checker.HasLockedShards() {
		return model.SQLResponse{}, ShardMigrationError{ShardID: 0}
	}

	groupIDs := groupIDsFromConfig(c.configReader.CurrentConfig())
	if len(groupIDs) == 0 {
		return model.SQLResponse{}, ErrNoShardNodesAvailable
	}

	var merged model.SQLResponse
	seenColumns := false
	rows := make([][]any, 0)
	for _, groupID := range groupIDs {
		node, err := c.pickGroupNode(ctx, groupID)
		if err != nil {
			return model.SQLResponse{}, err
		}
		response, err := c.executeRemoteSQL(ctx, node.HTTPAddr, input)
		if err != nil {
			return model.SQLResponse{}, err
		}
		if !seenColumns {
			merged = response
			seenColumns = true
		}
		rows = append(rows, response.Result.Rows...)
	}
	if !seenColumns {
		return model.SQLResponse{Success: true, Result: model.QueryResult{Type: "select", Rows: rows}}, nil
	}
	merged.Result.Rows = rows
	return merged, nil
}

func (c *Coordinator) executeJoin(ctx context.Context, statement model.Statement) (model.SQLResponse, error) {
	if statement.Join == nil {
		return model.SQLResponse{}, errors.New("join clause is required")
	}
	if checker, ok := c.configReader.(ShardLockChecker); ok && checker.HasLockedShards() {
		return model.SQLResponse{}, ShardMigrationError{ShardID: 0}
	}
	if len(statement.Columns) != 1 || statement.Columns[0] != "*" {
		return model.SQLResponse{}, fmt.Errorf("JOIN currently supports only SELECT *")
	}
	if statement.Filter != nil {
		return model.SQLResponse{}, ErrJoinWhereUnsupported
	}

	leftSchema, err := c.schemaForTable(ctx, statement.Table)
	if err != nil {
		return model.SQLResponse{}, err
	}
	rightSchema, err := c.schemaForTable(ctx, statement.Join.Table)
	if err != nil {
		return model.SQLResponse{}, err
	}

	leftQuery := fmt.Sprintf("SELECT * FROM %s", statement.Table)
	rightQuery := fmt.Sprintf("SELECT * FROM %s", statement.Join.Table)
	leftResponse, err := c.scatterSelect(ctx, leftQuery)
	if err != nil {
		return model.SQLResponse{}, err
	}
	rightResponse, err := c.scatterSelect(ctx, rightQuery)
	if err != nil {
		return model.SQLResponse{}, err
	}

	leftIndex, err := schemaColumnIndex(leftSchema, statement.Join.Left.Column)
	if err != nil {
		return model.SQLResponse{}, err
	}
	rightIndex, err := schemaColumnIndex(rightSchema, statement.Join.Right.Column)
	if err != nil {
		return model.SQLResponse{}, err
	}

	joinedRows := joinRows(leftResponse.Result.Rows, rightResponse.Result.Rows, leftIndex, rightIndex)
	return model.SQLResponse{
		Success: true,
		Result: model.QueryResult{
			Type:    "join",
			Columns: prefixedColumns(statement.Table, leftSchema.Columns, statement.Join.Table, rightSchema.Columns),
			Rows:    joinedRows,
		},
	}, nil
}

func (c *Coordinator) broadcastSQL(ctx context.Context, input string) (model.SQLResponse, error) {
	groupIDs := groupIDsFromConfig(c.configReader.CurrentConfig())
	if len(groupIDs) == 0 {
		return model.SQLResponse{}, ErrNoShardNodesAvailable
	}

	var firstResponse model.SQLResponse
	for index, groupID := range groupIDs {
		node, err := c.pickGroupNode(ctx, groupID)
		if err != nil {
			return model.SQLResponse{}, err
		}
		response, err := c.executeRemoteSQL(ctx, node.HTTPAddr, input)
		if err != nil {
			return model.SQLResponse{}, err
		}
		if index == 0 {
			firstResponse = response
		}
	}
	return firstResponse, nil
}

func (c *Coordinator) broadcastShowTables(ctx context.Context, input string) (model.SQLResponse, error) {
	groupIDs := groupIDsFromConfig(c.configReader.CurrentConfig())
	if len(groupIDs) == 0 {
		return model.SQLResponse{}, ErrNoShardNodesAvailable
	}

	tableSet := make(map[string]bool)
	for _, groupID := range groupIDs {
		node, err := c.pickGroupNode(ctx, groupID)
		if err != nil {
			return model.SQLResponse{}, err
		}
		response, err := c.executeRemoteSQL(ctx, node.HTTPAddr, input)
		if err != nil {
			return model.SQLResponse{}, err
		}
		for _, table := range response.Result.Tables {
			tableSet[table] = true
		}
	}

	tables := make([]string, 0, len(tableSet))
	for table := range tableSet {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return model.SQLResponse{
		Success: true,
		Result: model.QueryResult{
			Type:   "show_tables",
			Tables: tables,
		},
	}, nil
}

func (c *Coordinator) pickGroupNode(ctx context.Context, groupID shardmeta.GroupID) (model.NodeInfo, error) {
	nodes, err := c.nodeLister.ListNodes(ctx)
	if err != nil {
		return model.NodeInfo{}, err
	}

	candidates := make([]model.NodeInfo, 0)
	for _, node := range nodes {
		if node.Role != string(shardmeta.RoleShardNode) {
			continue
		}
		if node.GroupID != string(groupID) || node.HTTPAddr == "" {
			continue
		}
		candidates = append(candidates, node)
	}
	if len(candidates) == 0 {
		return model.NodeInfo{}, fmt.Errorf("%w: %s", ErrNoShardNodesAvailable, groupID)
	}
	for _, node := range candidates {
		if node.IsLeader {
			return node, nil
		}
	}
	return candidates[0], nil
}

func (c *Coordinator) executeRemoteSQL(ctx context.Context, baseURL, statement string) (model.SQLResponse, error) {
	response, err := c.postSQL(ctx, baseURL, statement)
	if err != nil {
		return model.SQLResponse{}, err
	}
	if !response.Success && response.Leader != "" {
		return c.postSQL(ctx, response.Leader, statement)
	}
	if !response.Success {
		return model.SQLResponse{}, errors.New(response.Error)
	}
	return response, nil
}

func (c *Coordinator) postSQL(ctx context.Context, baseURL, statement string) (model.SQLResponse, error) {
	body, err := json.Marshal(model.SQLRequest{SQL: statement})
	if err != nil {
		return model.SQLResponse{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeURL(baseURL)+"/sql", bytes.NewReader(body))
	if err != nil {
		return model.SQLResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
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
	return parsed, nil
}

func (c *Coordinator) fetchSchema(ctx context.Context, baseURL, table string) (model.TableSchema, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizeURL(baseURL)+"/schema?table="+url.QueryEscape(table), nil)
	if err != nil {
		return model.TableSchema{}, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return model.TableSchema{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		payload, _ := io.ReadAll(response.Body)
		return model.TableSchema{}, errors.New(string(payload))
	}
	var schema model.TableSchema
	if err := json.NewDecoder(response.Body).Decode(&schema); err != nil {
		return model.TableSchema{}, err
	}
	return schema, nil
}

func (c *Coordinator) schemaForTable(ctx context.Context, table string) (model.TableSchema, error) {
	groupIDs := groupIDsFromConfig(c.configReader.CurrentConfig())
	if len(groupIDs) == 0 {
		return model.TableSchema{}, ErrNoShardNodesAvailable
	}
	var lastErr error
	for _, groupID := range groupIDs {
		node, err := c.pickGroupNode(ctx, groupID)
		if err != nil {
			lastErr = err
			continue
		}
		schema, err := c.fetchSchema(ctx, node.HTTPAddr, table)
		if err == nil {
			return schema, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return model.TableSchema{}, lastErr
	}
	return model.TableSchema{}, fmt.Errorf("table %s schema not found", table)
}

func (c *Coordinator) ensureTable(ctx context.Context, baseURL string, schema model.TableSchema) error {
	statement := buildCreateTableSQL(schema)
	response, err := c.postSQL(ctx, baseURL, statement)
	if err != nil {
		return err
	}
	if !response.Success && !strings.Contains(response.Error, "already exists") {
		return errors.New(response.Error)
	}
	return nil
}

func (c *Coordinator) insertRow(ctx context.Context, baseURL, table string, row []any) error {
	response, err := c.postSQL(ctx, baseURL, buildInsertSQL(table, row))
	if err != nil {
		return err
	}
	if !response.Success && !strings.Contains(response.Error, "already exists") {
		return errors.New(response.Error)
	}
	return nil
}

func (c *Coordinator) deleteRow(ctx context.Context, baseURL, table, primaryKey string, value any) error {
	response, err := c.postSQL(ctx, baseURL, fmt.Sprintf("DELETE FROM %s WHERE %s = %s", table, primaryKey, sqlLiteral(value)))
	if err != nil {
		return err
	}
	if !response.Success {
		return errors.New(response.Error)
	}
	return nil
}

func groupIDsFromConfig(config shardmeta.ClusterConfig) []shardmeta.GroupID {
	seen := make(map[shardmeta.GroupID]bool)
	groupIDs := make([]shardmeta.GroupID, 0, len(config.Assignments))
	for _, assignment := range config.Assignments {
		if seen[assignment.GroupID] {
			continue
		}
		seen[assignment.GroupID] = true
		groupIDs = append(groupIDs, assignment.GroupID)
	}
	slices.Sort(groupIDs)
	return groupIDs
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

func buildCreateTableSQL(schema model.TableSchema) string {
	parts := make([]string, 0, len(schema.Columns))
	for _, column := range schema.Columns {
		definition := column.Name + " " + column.Type
		if column.PrimaryKey {
			definition += " PRIMARY KEY"
		}
		parts = append(parts, definition)
	}
	return fmt.Sprintf("CREATE TABLE %s (%s)", schema.Name, strings.Join(parts, ", "))
}

func buildInsertSQL(table string, row []any) string {
	values := make([]string, 0, len(row))
	for _, value := range row {
		values = append(values, sqlLiteral(value))
	}
	return fmt.Sprintf("INSERT INTO %s VALUES (%s)", table, strings.Join(values, ", "))
}

func sqlLiteral(value any) string {
	switch typed := value.(type) {
	case string:
		return "'" + strings.ReplaceAll(typed, "'", "''") + "'"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func primaryKeyIndex(schema model.TableSchema) int {
	for index, column := range schema.Columns {
		if column.Name == schema.PrimaryKey {
			return index
		}
	}
	return -1
}

func schemaColumnIndex(schema model.TableSchema, column string) (int, error) {
	for index, item := range schema.Columns {
		if strings.EqualFold(item.Name, column) {
			return index, nil
		}
	}
	return -1, fmt.Errorf("unknown join column %s on table %s", column, schema.Name)
}

func joinRows(leftRows, rightRows [][]any, leftIndex, rightIndex int) [][]any {
	rightBuckets := make(map[string][][]any)
	for _, row := range rightRows {
		if rightIndex >= len(row) {
			continue
		}
		key := normalizeJoinValue(row[rightIndex])
		rightBuckets[key] = append(rightBuckets[key], row)
	}

	joined := make([][]any, 0)
	for _, leftRow := range leftRows {
		if leftIndex >= len(leftRow) {
			continue
		}
		for _, rightRow := range rightBuckets[normalizeJoinValue(leftRow[leftIndex])] {
			row := make([]any, 0, len(leftRow)+len(rightRow))
			row = append(row, leftRow...)
			row = append(row, rightRow...)
			joined = append(joined, row)
		}
	}
	return joined
}

func prefixedColumns(leftTable string, leftColumns []model.ColumnDef, rightTable string, rightColumns []model.ColumnDef) []string {
	columns := make([]string, 0, len(leftColumns)+len(rightColumns))
	for _, column := range leftColumns {
		columns = append(columns, leftTable+"."+column.Name)
	}
	for _, column := range rightColumns {
		columns = append(columns, rightTable+"."+column.Name)
	}
	return columns
}

func normalizeJoinValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	default:
		return fmt.Sprintf("%v", typed)
	}
}
