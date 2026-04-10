package sql

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yedou37/dbd/internal/model"
)

func Parse(input string) (model.Statement, error) {
	sql := strings.TrimSpace(strings.TrimSuffix(input, ";"))
	upper := strings.ToUpper(sql)

	switch {
	case strings.HasPrefix(upper, "CREATE TABLE "):
		return parseCreateTable(sql)
	case strings.HasPrefix(upper, "INSERT INTO "):
		return parseInsert(sql)
	case strings.HasPrefix(upper, "SELECT "):
		return parseSelect(sql)
	case strings.HasPrefix(upper, "DELETE FROM "):
		return parseDelete(sql)
	case upper == "SHOW TABLES":
		return model.Statement{Type: model.StatementShowTables, Raw: sql}, nil
	default:
		return model.Statement{}, fmt.Errorf("unsupported SQL: %s", sql)
	}
}

func parseCreateTable(sql string) (model.Statement, error) {
	body := strings.TrimSpace(sql[len("CREATE TABLE "):])
	open := strings.Index(body, "(")
	close := strings.LastIndex(body, ")")
	if open <= 0 || close <= open {
		return model.Statement{}, fmt.Errorf("invalid CREATE TABLE syntax")
	}

	table := strings.TrimSpace(body[:open])
	rawColumns := splitCommaAware(body[open+1 : close])
	definitions := make([]model.ColumnDef, 0, len(rawColumns))
	var primaryKey string

	for _, rawColumn := range rawColumns {
		parts := strings.Fields(strings.TrimSpace(rawColumn))
		if len(parts) < 2 {
			return model.Statement{}, fmt.Errorf("invalid column definition: %s", rawColumn)
		}

		column := model.ColumnDef{
			Name: parts[0],
			Type: strings.ToUpper(parts[1]),
		}
		if len(parts) >= 4 && strings.EqualFold(parts[2], "PRIMARY") && strings.EqualFold(parts[3], "KEY") {
			column.PrimaryKey = true
			primaryKey = column.Name
		}
		definitions = append(definitions, column)
	}

	return model.Statement{
		Type:        model.StatementCreateTable,
		Table:       table,
		Definitions: definitions,
		Raw:         sql,
	}, setPrimaryKey(primaryKey)
}

func parseInsert(sql string) (model.Statement, error) {
	body := strings.TrimSpace(sql[len("INSERT INTO "):])
	parts := strings.SplitN(body, "VALUES", 2)
	if len(parts) != 2 {
		return model.Statement{}, fmt.Errorf("invalid INSERT syntax")
	}

	table := strings.TrimSpace(parts[0])
	valuesBlock := strings.TrimSpace(parts[1])
	if !strings.HasPrefix(valuesBlock, "(") || !strings.HasSuffix(valuesBlock, ")") {
		return model.Statement{}, fmt.Errorf("invalid INSERT values")
	}

	rawValues := splitCommaAware(valuesBlock[1 : len(valuesBlock)-1])
	values := make([]any, 0, len(rawValues))
	for _, raw := range rawValues {
		value, err := parseLiteral(raw)
		if err != nil {
			return model.Statement{}, err
		}
		values = append(values, value)
	}

	return model.Statement{
		Type:   model.StatementInsert,
		Table:  table,
		Values: values,
		Raw:    sql,
	}, nil
}

func parseSelect(sql string) (model.Statement, error) {
	body := strings.TrimSpace(sql[len("SELECT "):])
	fromIndex := strings.Index(strings.ToUpper(body), " FROM ")
	if fromIndex == -1 {
		return model.Statement{}, fmt.Errorf("invalid SELECT syntax")
	}

	columnPart := strings.TrimSpace(body[:fromIndex])
	rest := strings.TrimSpace(body[fromIndex+len(" FROM "):])
	columns := splitCommaAware(columnPart)

	table := rest
	var filter *model.Filter
	whereIndex := strings.Index(strings.ToUpper(rest), " WHERE ")
	if whereIndex >= 0 {
		table = strings.TrimSpace(rest[:whereIndex])
		parsedFilter, err := parseWhere(rest[whereIndex+len(" WHERE "):])
		if err != nil {
			return model.Statement{}, err
		}
		filter = parsedFilter
	}

	return model.Statement{
		Type:    model.StatementSelect,
		Table:   table,
		Columns: columns,
		Filter:  filter,
		Raw:     sql,
	}, nil
}

func parseDelete(sql string) (model.Statement, error) {
	body := strings.TrimSpace(sql[len("DELETE FROM "):])
	whereIndex := strings.Index(strings.ToUpper(body), " WHERE ")
	if whereIndex == -1 {
		return model.Statement{}, fmt.Errorf("DELETE must include WHERE")
	}

	table := strings.TrimSpace(body[:whereIndex])
	filter, err := parseWhere(body[whereIndex+len(" WHERE "):])
	if err != nil {
		return model.Statement{}, err
	}

	return model.Statement{
		Type:   model.StatementDelete,
		Table:  table,
		Filter: filter,
		Raw:    sql,
	}, nil
}

func parseWhere(input string) (*model.Filter, error) {
	parts := strings.SplitN(input, "=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("only simple equality WHERE is supported")
	}

	value, err := parseLiteral(parts[1])
	if err != nil {
		return nil, err
	}

	return &model.Filter{
		Column: strings.TrimSpace(parts[0]),
		Value:  value,
	}, nil
}

func parseLiteral(input string) (any, error) {
	value := strings.TrimSpace(input)
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1], nil
	}

	if number, err := strconv.Atoi(value); err == nil {
		return number, nil
	}

	if number, err := strconv.ParseFloat(value, 64); err == nil {
		return number, nil
	}

	return value, nil
}

func splitCommaAware(input string) []string {
	parts := make([]string, 0)
	current := strings.Builder{}
	inQuote := false

	for _, char := range input {
		switch char {
		case '\'':
			inQuote = !inQuote
			current.WriteRune(char)
		case ',':
			if inQuote {
				current.WriteRune(char)
				continue
			}
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, strings.TrimSpace(current.String()))
	}

	return parts
}

func setPrimaryKey(primaryKey string) error {
	if primaryKey == "" {
		return fmt.Errorf("CREATE TABLE requires one PRIMARY KEY column")
	}
	return nil
}
