package model

import (
	"encoding/json"
	"testing"
)

func TestSQLResponseJSONRoundTrip(t *testing.T) {
	original := SQLResponse{
		Success: true,
		Result: QueryResult{
			Type:    "select",
			Columns: []string{"id", "name"},
			Rows:    [][]any{{1, "alice"}},
			Schema: &TableSchema{
				Name: "users",
				Columns: []ColumnDef{
					{Name: "id", Type: "INT", PrimaryKey: true},
					{Name: "name", Type: "TEXT"},
				},
				PrimaryKey: "id",
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded SQLResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if !decoded.Success || decoded.Result.Type != "select" {
		t.Fatalf("decoded = %#v", decoded)
	}
}
