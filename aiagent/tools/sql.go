package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/toolkits/pkg/logger"
)

type columnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment,omitempty"`
}

func init() {
	register("list_databases", aiagent.AgentTool{
		Name:        "list_databases",
		Description: "列出 SQL 数据源（MySQL/Doris/ClickHouse/PostgreSQL）中的所有数据库",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters:  []aiagent.ToolParameter{},
	}, listDatabasesTool)

	register("list_tables", aiagent.AgentTool{
		Name:        "list_tables",
		Description: "列出指定数据库中的所有表",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "database", Type: "string", Description: "数据库名", Required: true},
		},
	}, listTablesTool)

	register("describe_table", aiagent.AgentTool{
		Name:        "describe_table",
		Description: "获取表的字段结构（字段名、类型、注释）",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "database", Type: "string", Description: "数据库名", Required: true},
			{Name: "table", Type: "string", Description: "表名", Required: true},
		},
	}, describeTableTool)
}

func listDatabasesTool(ctx context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	dsId := getDatasourceId(params)
	dsType := getDatasourceType(params)
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id not found in params")
	}
	if dsType == "" {
		return "", fmt.Errorf("datasource_type not found in params")
	}

	plug, exists := aiagent.GetSQLDatasource(dsType, dsId)
	if !exists {
		return "", fmt.Errorf("datasource not found: %s/%d", dsType, dsId)
	}

	var sql string
	switch dsType {
	case "mysql", "doris":
		sql = "SHOW DATABASES"
	case "ck", "clickhouse":
		sql = "SHOW DATABASES"
	case "pgsql", "postgresql":
		sql = "SELECT datname FROM pg_database WHERE datistemplate = false"
	default:
		return "", fmt.Errorf("unsupported datasource type for list_databases: %s", dsType)
	}

	query := map[string]interface{}{"sql": sql}
	data, _, err := plug.QueryLog(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to list databases: %v", err)
	}

	databases := extractColumnValues(data, "database")
	logger.Debugf("list_databases: dsType=%s, found %d databases", dsType, len(databases))

	bytes, _ := json.Marshal(databases)
	return string(bytes), nil
}

func listTablesTool(ctx context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	dsId := getDatasourceId(params)
	dsType := getDatasourceType(params)
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id not found in params")
	}

	database, ok := args["database"].(string)
	if !ok || database == "" {
		return "", fmt.Errorf("database parameter is required")
	}
	if !isValidIdentifier(database) {
		return "", fmt.Errorf("invalid database name: %s", database)
	}

	plug, exists := aiagent.GetSQLDatasource(dsType, dsId)
	if !exists {
		return "", fmt.Errorf("datasource not found: %s/%d", dsType, dsId)
	}

	var sql string
	switch dsType {
	case "mysql", "doris":
		sql = fmt.Sprintf("SHOW TABLES FROM `%s`", database)
	case "ck", "clickhouse":
		sql = fmt.Sprintf("SHOW TABLES FROM `%s`", database)
	case "pgsql", "postgresql":
		sql = "SELECT tablename FROM pg_tables WHERE schemaname = 'public'"
	default:
		return "", fmt.Errorf("unsupported datasource type for list_tables: %s", dsType)
	}

	query := map[string]interface{}{"sql": sql, "database": database}
	data, _, err := plug.QueryLog(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to list tables: %v", err)
	}

	tables := extractColumnValues(data, "table")
	logger.Debugf("list_tables: dsType=%s, database=%s, found %d tables", dsType, database, len(tables))

	bytes, _ := json.Marshal(tables)
	return string(bytes), nil
}

func describeTableTool(ctx context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	dsId := getDatasourceId(params)
	dsType := getDatasourceType(params)
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id not found in params")
	}

	database, ok := args["database"].(string)
	if !ok || database == "" {
		return "", fmt.Errorf("database parameter is required")
	}
	if !isValidIdentifier(database) {
		return "", fmt.Errorf("invalid database name: %s", database)
	}
	table, ok := args["table"].(string)
	if !ok || table == "" {
		return "", fmt.Errorf("table parameter is required")
	}
	if !isValidIdentifier(table) {
		return "", fmt.Errorf("invalid table name: %s", table)
	}

	plug, exists := aiagent.GetSQLDatasource(dsType, dsId)
	if !exists {
		return "", fmt.Errorf("datasource not found: %s/%d", dsType, dsId)
	}

	var sql string
	switch dsType {
	case "mysql", "doris":
		sql = fmt.Sprintf("DESCRIBE `%s`.`%s`", database, table)
	case "ck", "clickhouse":
		sql = fmt.Sprintf("DESCRIBE TABLE `%s`.`%s`", database, table)
	case "pgsql", "postgresql":
		sql = fmt.Sprintf(`SELECT column_name as "Field", data_type as "Type", is_nullable as "Null", column_default as "Default" FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '%s'`, table)
	default:
		return "", fmt.Errorf("unsupported datasource type for describe_table: %s", dsType)
	}

	query := map[string]interface{}{"sql": sql, "database": database}
	data, _, err := plug.QueryLog(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to describe table: %v", err)
	}

	columns := convertToColumnInfo(data)
	logger.Debugf("describe_table: dsType=%s, table=%s.%s, found %d columns", dsType, database, table, len(columns))

	bytes, _ := json.Marshal(columns)
	return string(bytes), nil
}

func extractColumnValues(data []interface{}, columnType string) []string {
	possible := map[string][]string{
		"database": {"Database", "database", "datname", "name"},
		"table":    {"Tables_in_", "table", "tablename", "name", "Name"},
	}
	keys := possible[columnType]

	result := make([]string, 0)
	for _, row := range data {
		if rowMap, ok := row.(map[string]interface{}); ok {
			var value string
			for _, key := range keys {
				if v, ok := rowMap[key]; ok {
					if s, ok := v.(string); ok {
						value = s
						break
					}
				}
			}
			if value != "" {
				result = append(result, value)
			}
		}
	}
	return result
}

func convertToColumnInfo(data []interface{}) []columnInfo {
	result := make([]columnInfo, 0)
	for _, row := range data {
		if rowMap, ok := row.(map[string]interface{}); ok {
			col := columnInfo{}
			for _, key := range []string{"Field", "field", "column_name", "name"} {
				if v, ok := rowMap[key]; ok {
					if s, ok := v.(string); ok {
						col.Name = s
						break
					}
				}
			}
			for _, key := range []string{"Type", "type", "data_type"} {
				if v, ok := rowMap[key]; ok {
					if s, ok := v.(string); ok {
						col.Type = s
						break
					}
				}
			}
			for _, key := range []string{"Comment", "comment", "column_comment"} {
				if v, ok := rowMap[key]; ok {
					if s, ok := v.(string); ok {
						col.Comment = s
						break
					}
				}
			}
			if col.Name != "" {
				result = append(result, col)
			}
		}
	}
	return result
}
