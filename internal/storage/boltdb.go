package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	bolt "go.etcd.io/bbolt"

	"github.com/yedou37/dbd/internal/model"
)

const schemasBucket = "__schemas"

type Store struct {
	db *bolt.DB
}

func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(schemasBucket))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) ExecuteStatement(statement model.Statement) (model.QueryResult, error) {
	switch statement.Type {
	case model.StatementCreateTable:
		schema := model.TableSchema{
			Name:    statement.Table,
			Columns: statement.Definitions,
		}
		for _, column := range statement.Definitions {
			if column.PrimaryKey {
				schema.PrimaryKey = column.Name
				break
			}
		}
		if err := s.CreateTable(schema); err != nil {
			return model.QueryResult{}, err
		}
		return model.QueryResult{Type: "create_table", Schema: &schema}, nil
	case model.StatementInsert:
		if err := s.Insert(statement.Table, statement.Values); err != nil {
			return model.QueryResult{}, err
		}
		return model.QueryResult{Type: "insert", RowsAffected: 1}, nil
	case model.StatementSelect:
		return s.Select(statement.Table, statement.Columns, statement.Filter)
	case model.StatementDelete:
		rows, err := s.Delete(statement.Table, statement.Filter)
		if err != nil {
			return model.QueryResult{}, err
		}
		return model.QueryResult{Type: "delete", RowsAffected: rows}, nil
	case model.StatementShowTables:
		tables, err := s.ListTables()
		if err != nil {
			return model.QueryResult{}, err
		}
		return model.QueryResult{Type: "show_tables", Tables: tables}, nil
	default:
		return model.QueryResult{}, fmt.Errorf("unsupported statement type %s", statement.Type)
	}
}

func (s *Store) CreateTable(schema model.TableSchema) error {
	if schema.Name == "" {
		return errors.New("table name is required")
	}
	if len(schema.Columns) == 0 {
		return errors.New("table must have at least one column")
	}
	if schema.PrimaryKey == "" {
		return errors.New("table must declare a primary key")
	}

	data, err := json.Marshal(schema)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		schemas := tx.Bucket([]byte(schemasBucket))
		if schemas.Get([]byte(schema.Name)) != nil {
			return fmt.Errorf("table %s already exists", schema.Name)
		}
		if _, err := tx.CreateBucket([]byte(schema.Name)); err != nil {
			return err
		}
		return schemas.Put([]byte(schema.Name), data)
	})
}

func (s *Store) ListTables() ([]string, error) {
	var tables []string
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(schemasBucket)).ForEach(func(k, _ []byte) error {
			tables = append(tables, string(k))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(tables)
	return tables, nil
}

func (s *Store) Insert(table string, values []any) error {
	schema, err := s.loadSchema(table)
	if err != nil {
		return err
	}
	if len(values) != len(schema.Columns) {
		return fmt.Errorf("expected %d values, got %d", len(schema.Columns), len(values))
	}

	row := make(map[string]any, len(schema.Columns))
	for index, column := range schema.Columns {
		row[column.Name] = values[index]
	}

	primaryValue, ok := row[schema.PrimaryKey]
	if !ok {
		return fmt.Errorf("primary key %s is missing", schema.PrimaryKey)
	}

	key := []byte(normalizeValue(primaryValue))
	data, err := json.Marshal(row)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(table))
		if bucket.Get(key) != nil {
			return fmt.Errorf("row with primary key %s already exists", string(key))
		}
		return bucket.Put(key, data)
	})
}

func (s *Store) Select(table string, columns []string, filter *model.Filter) (model.QueryResult, error) {
	schema, err := s.loadSchema(table)
	if err != nil {
		return model.QueryResult{}, err
	}

	selectedColumns := columns
	if len(selectedColumns) == 0 || (len(selectedColumns) == 1 && selectedColumns[0] == "*") {
		selectedColumns = schemaColumnNames(schema)
	}
	if err := validateColumns(schema, selectedColumns); err != nil {
		return model.QueryResult{}, err
	}
	if filter != nil && !hasColumn(schema, filter.Column) {
		return model.QueryResult{}, fmt.Errorf("unknown filter column %s", filter.Column)
	}

	result := model.QueryResult{Type: "select", Columns: selectedColumns, Rows: make([][]any, 0)}
	err = s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(table))
		return bucket.ForEach(func(_, value []byte) error {
			var row map[string]any
			if err := json.Unmarshal(value, &row); err != nil {
				return err
			}
			if filter != nil && normalizeValue(row[filter.Column]) != normalizeValue(filter.Value) {
				return nil
			}
			selectedRow := make([]any, 0, len(selectedColumns))
			for _, column := range selectedColumns {
				selectedRow = append(selectedRow, row[column])
			}
			result.Rows = append(result.Rows, selectedRow)
			return nil
		})
	})
	return result, err
}

func (s *Store) Delete(table string, filter *model.Filter) (int, error) {
	if filter == nil {
		return 0, errors.New("delete requires a where clause")
	}

	schema, err := s.loadSchema(table)
	if err != nil {
		return 0, err
	}
	if !hasColumn(schema, filter.Column) {
		return 0, fmt.Errorf("unknown filter column %s", filter.Column)
	}

	removed := 0
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(table))
		toDelete := make([][]byte, 0)
		if err := bucket.ForEach(func(key, value []byte) error {
			var row map[string]any
			if err := json.Unmarshal(value, &row); err != nil {
				return err
			}
			if normalizeValue(row[filter.Column]) == normalizeValue(filter.Value) {
				toDelete = append(toDelete, append([]byte(nil), key...))
			}
			return nil
		}); err != nil {
			return err
		}
		for _, key := range toDelete {
			if err := bucket.Delete(key); err != nil {
				return err
			}
			removed++
		}
		return nil
	})
	return removed, err
}

func (s *Store) loadSchema(table string) (model.TableSchema, error) {
	var schema model.TableSchema
	err := s.db.View(func(tx *bolt.Tx) error {
		schemas := tx.Bucket([]byte(schemasBucket))
		data := schemas.Get([]byte(table))
		if data == nil {
			return fmt.Errorf("table %s does not exist", table)
		}
		return json.Unmarshal(data, &schema)
	})
	return schema, err
}

func schemaColumnNames(schema model.TableSchema) []string {
	columns := make([]string, 0, len(schema.Columns))
	for _, column := range schema.Columns {
		columns = append(columns, column.Name)
	}
	return columns
}

func validateColumns(schema model.TableSchema, columns []string) error {
	for _, column := range columns {
		if !hasColumn(schema, column) {
			return fmt.Errorf("unknown column %s", column)
		}
	}
	return nil
}

func hasColumn(schema model.TableSchema, name string) bool {
	for _, column := range schema.Columns {
		if strings.EqualFold(column.Name, name) {
			return true
		}
	}
	return false
}

func normalizeValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.6f", typed), "000000"), ".")
	default:
		return fmt.Sprintf("%v", typed)
	}
}
