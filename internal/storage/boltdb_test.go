package storage

import (
	"path/filepath"
	"testing"

	"github.com/yedou37/ddb/internal/model"
)

func TestStoreCRUDFlow(t *testing.T) {
	store := openTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	schema := model.TableSchema{
		Name: "users",
		Columns: []model.ColumnDef{
			{Name: "id", Type: "INT", PrimaryKey: true},
			{Name: "name", Type: "TEXT"},
			{Name: "age", Type: "INT"},
		},
		PrimaryKey: "id",
	}

	if err := store.CreateTable(schema); err != nil {
		t.Fatalf("CreateTable() error = %v", err)
	}
	if err := store.Insert("users", []any{1, "alice", 18}); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if err := store.Insert("users", []any{2, "bob", 20}); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	result, err := store.Select("users", []string{"id", "name"}, &model.Filter{Column: "id", Value: 1}, nil, nil)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got, want := len(result.Rows), 1; got != want {
		t.Fatalf("len(result.Rows) = %d, want %d", got, want)
	}
	if got, want := result.Rows[0][1], "alice"; got != want {
		t.Fatalf("result.Rows[0][1] = %#v, want %#v", got, want)
	}

	removed, err := store.Delete("users", &model.Filter{Column: "id", Value: 1})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got, want := removed, 1; got != want {
		t.Fatalf("removed = %d, want %d", got, want)
	}

	result, err = store.Select("users", []string{"*"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Select() after delete error = %v", err)
	}
	if got, want := len(result.Rows), 1; got != want {
		t.Fatalf("len(result.Rows) after delete = %d, want %d", got, want)
	}
	if got, want := result.Rows[0][1], "bob"; got != want {
		t.Fatalf("remaining row = %#v, want bob row", result.Rows[0])
	}
}

func TestStoreListTablesSorted(t *testing.T) {
	store := openTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	for _, schema := range []model.TableSchema{
		{
			Name: "z_table",
			Columns: []model.ColumnDef{
				{Name: "id", Type: "INT", PrimaryKey: true},
			},
			PrimaryKey: "id",
		},
		{
			Name: "a_table",
			Columns: []model.ColumnDef{
				{Name: "id", Type: "INT", PrimaryKey: true},
			},
			PrimaryKey: "id",
		},
	} {
		if err := store.CreateTable(schema); err != nil {
			t.Fatalf("CreateTable(%s) error = %v", schema.Name, err)
		}
	}

	tables, err := store.ListTables()
	if err != nil {
		t.Fatalf("ListTables() error = %v", err)
	}
	if got, want := len(tables), 2; got != want {
		t.Fatalf("len(tables) = %d, want %d", got, want)
	}
	if tables[0] != "a_table" || tables[1] != "z_table" {
		t.Fatalf("tables = %#v, want sorted order", tables)
	}
}

func TestStoreDropTable(t *testing.T) {
	store := openTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	schema := model.TableSchema{
		Name: "users",
		Columns: []model.ColumnDef{
			{Name: "id", Type: "INT", PrimaryKey: true},
			{Name: "name", Type: "TEXT"},
		},
		PrimaryKey: "id",
	}
	if err := store.CreateTable(schema); err != nil {
		t.Fatalf("CreateTable() error = %v", err)
	}

	if err := store.DropTable("users"); err != nil {
		t.Fatalf("DropTable() error = %v", err)
	}
	if _, err := store.Schema("users"); err == nil {
		t.Fatalf("Schema() error = nil, want error after drop")
	}
}

func TestStoreSelectOrderByAndLimit(t *testing.T) {
	store := openTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	if err := store.CreateTable(model.TableSchema{
		Name: "users",
		Columns: []model.ColumnDef{
			{Name: "id", Type: "INT", PrimaryKey: true},
			{Name: "name", Type: "TEXT"},
		},
		PrimaryKey: "id",
	}); err != nil {
		t.Fatalf("CreateTable() error = %v", err)
	}
	for _, row := range [][]any{{3, "c"}, {1, "a"}, {2, "b"}} {
		if err := store.Insert("users", row); err != nil {
			t.Fatalf("Insert(%v) error = %v", row, err)
		}
	}

	limit := 2
	result, err := store.Select("users", []string{"id", "name"}, nil, &model.OrderBy{Column: "id", Desc: true}, &limit)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got, want := len(result.Rows), 2; got != want {
		t.Fatalf("len(result.Rows) = %d, want %d", got, want)
	}
	if got, want := result.Rows[0][0], 3.0; got != want {
		t.Fatalf("result.Rows[0][0] = %#v, want %#v", got, want)
	}
	if got, want := result.Rows[1][0], 2.0; got != want {
		t.Fatalf("result.Rows[1][0] = %#v, want %#v", got, want)
	}
}

func TestStoreRejectsDuplicatePrimaryKey(t *testing.T) {
	store := openTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	if err := store.CreateTable(model.TableSchema{
		Name: "users",
		Columns: []model.ColumnDef{
			{Name: "id", Type: "INT", PrimaryKey: true},
			{Name: "name", Type: "TEXT"},
		},
		PrimaryKey: "id",
	}); err != nil {
		t.Fatalf("CreateTable() error = %v", err)
	}

	if err := store.Insert("users", []any{1, "alice"}); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if err := store.Insert("users", []any{1, "duplicate"}); err == nil {
		t.Fatalf("Insert() duplicate error = nil, want error")
	}
}

func TestExecuteStatementShowTables(t *testing.T) {
	store := openTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	if _, err := store.ExecuteStatement(model.Statement{
		Type:  model.StatementCreateTable,
		Table: "books",
		Definitions: []model.ColumnDef{
			{Name: "id", Type: "INT", PrimaryKey: true},
			{Name: "name", Type: "TEXT"},
		},
	}); err != nil {
		t.Fatalf("ExecuteStatement(create) error = %v", err)
	}

	result, err := store.ExecuteStatement(model.Statement{Type: model.StatementShowTables})
	if err != nil {
		t.Fatalf("ExecuteStatement(show tables) error = %v", err)
	}
	if got, want := len(result.Tables), 1; got != want {
		t.Fatalf("len(result.Tables) = %d, want %d", got, want)
	}
	if got, want := result.Tables[0], "books"; got != want {
		t.Fatalf("result.Tables[0] = %q, want %q", got, want)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return store
}
