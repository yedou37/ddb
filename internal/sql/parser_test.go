package sql

import (
	"testing"

	"github.com/yedou37/ddb/internal/model"
)

func TestParseCreateTable(t *testing.T) {
	statement, err := Parse("CREATE TABLE users (id INT PRIMARY KEY, name TEXT, age INT)")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if statement.Type != model.StatementCreateTable {
		t.Fatalf("statement.Type = %q, want %q", statement.Type, model.StatementCreateTable)
	}
	if statement.Table != "users" {
		t.Fatalf("statement.Table = %q, want users", statement.Table)
	}
	if len(statement.Definitions) != 3 {
		t.Fatalf("len(statement.Definitions) = %d, want 3", len(statement.Definitions))
	}
	if !statement.Definitions[0].PrimaryKey {
		t.Fatalf("first column should be primary key")
	}
}

func TestParseInsertWithQuotedComma(t *testing.T) {
	statement, err := Parse("INSERT INTO users VALUES (1, 'Alice, Bob', 18)")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if statement.Type != model.StatementInsert {
		t.Fatalf("statement.Type = %q, want %q", statement.Type, model.StatementInsert)
	}
	if got, want := len(statement.Values), 3; got != want {
		t.Fatalf("len(statement.Values) = %d, want %d", got, want)
	}
	if got, want := statement.Values[1], "Alice, Bob"; got != want {
		t.Fatalf("statement.Values[1] = %#v, want %#v", got, want)
	}
}

func TestParseSelectWithWhere(t *testing.T) {
	statement, err := Parse("SELECT id, name FROM users WHERE id = 1")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if statement.Type != model.StatementSelect {
		t.Fatalf("statement.Type = %q, want %q", statement.Type, model.StatementSelect)
	}
	if got, want := statement.Columns, []string{"id", "name"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("statement.Columns = %#v, want %#v", got, want)
	}
	if statement.Filter == nil {
		t.Fatalf("statement.Filter should not be nil")
	}
	if statement.Filter.Column != "id" || statement.Filter.Value != 1 {
		t.Fatalf("statement.Filter = %#v, want column=id value=1", statement.Filter)
	}
}

func TestParseDeleteRequiresWhere(t *testing.T) {
	_, err := Parse("DELETE FROM users")
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}

func TestParseShowTables(t *testing.T) {
	statement, err := Parse("SHOW TABLES")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if statement.Type != model.StatementShowTables {
		t.Fatalf("statement.Type = %q, want %q", statement.Type, model.StatementShowTables)
	}
}

func TestParseCreateTableRequiresPrimaryKey(t *testing.T) {
	_, err := Parse("CREATE TABLE users (id INT, name TEXT)")
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}
