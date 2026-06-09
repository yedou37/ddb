package sql

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/yedou37/ddb/internal/model"
)

func MaterializeWriteStatement(statement model.Statement) (model.Statement, error) {
	if statement.Type != model.StatementInsert {
		return statement, nil
	}

	values := make([]any, len(statement.Values))
	changed := false
	for index, value := range statement.Values {
		switch typed := value.(type) {
		case model.FunctionCall:
			resolved, err := materializeFunctionCall(typed)
			if err != nil {
				return model.Statement{}, err
			}
			values[index] = resolved
			changed = true
		default:
			values[index] = value
		}
	}

	if !changed {
		return statement, nil
	}

	statement.Values = values
	statement.Raw = BuildInsertSQL(statement.Table, values)
	return statement, nil
}

func materializeFunctionCall(call model.FunctionCall) (any, error) {
	switch {
	case strings.EqualFold(call.Name, "RAND"):
		return rand.Float64(), nil
	default:
		return nil, fmt.Errorf("unsupported function %s()", call.Name)
	}
}

func BuildInsertSQL(table string, values []any) string {
	literals := make([]string, 0, len(values))
	for _, value := range values {
		literals = append(literals, sqlLiteral(value))
	}
	return fmt.Sprintf("INSERT INTO %s VALUES (%s)", table, strings.Join(literals, ", "))
}

func sqlLiteral(value any) string {
	switch typed := value.(type) {
	case string:
		return "'" + strings.ReplaceAll(typed, "'", "''") + "'"
	case int:
		return strconv.Itoa(typed)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", typed)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", typed)
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", typed)
	}
}
