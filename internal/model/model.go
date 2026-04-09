package model

type NodeInfo struct {
	ID       string `json:"id"`
	RaftAddr string `json:"raft_addr,omitempty"`
	HTTPAddr string `json:"http_addr"`
	IsLeader bool   `json:"is_leader"`
}

type ColumnDef struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	PrimaryKey bool   `json:"primary_key"`
}

type TableSchema struct {
	Name       string      `json:"name"`
	Columns    []ColumnDef `json:"columns"`
	PrimaryKey string      `json:"primary_key"`
}

type Filter struct {
	Column string `json:"column"`
	Value  any    `json:"value"`
}

type StatementType string

const (
	StatementCreateTable StatementType = "create_table"
	StatementInsert      StatementType = "insert"
	StatementSelect      StatementType = "select"
	StatementDelete      StatementType = "delete"
	StatementShowTables  StatementType = "show_tables"
)

type Statement struct {
	Type        StatementType
	Table       string
	Columns     []string
	Definitions []ColumnDef
	Values      []any
	Filter      *Filter
	Raw         string
}

type QueryResult struct {
	Type         string       `json:"type"`
	Columns      []string     `json:"columns,omitempty"`
	Rows         [][]any      `json:"rows,omitempty"`
	RowsAffected int          `json:"rows_affected,omitempty"`
	Tables       []string     `json:"tables,omitempty"`
	Schema       *TableSchema `json:"schema,omitempty"`
}

type SQLRequest struct {
	SQL string `json:"sql"`
}

type JoinRequest struct {
	NodeID   string `json:"node_id"`
	RaftAddr string `json:"raft_addr"`
	HTTPAddr string `json:"http_addr"`
}

type SQLResponse struct {
	Success bool        `json:"success"`
	Result  QueryResult `json:"result,omitempty"`
	Error   string      `json:"error,omitempty"`
	Leader  string      `json:"leader,omitempty"`
}

type StatusResponse struct {
	NodeID   string   `json:"node_id"`
	HTTPAddr string   `json:"http_addr"`
	Role     string   `json:"role"`
	Leader   string   `json:"leader"`
	Tables   []string `json:"tables"`
}
