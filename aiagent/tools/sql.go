package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type columnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment,omitempty"`
}

func init() {
	register(defs.ListDatabases, listDatabasesTool)
	register(defs.ListTables, listTablesTool)
	register(defs.DescribeTable, describeTableTool)
}

// resolveSQLDatasource picks a (datasource_id, plugin_type) pair from the
// three possible sources, in order of precedence:
//
//  1. explicit tool args — the caller knows exactly which datasource to
//     target. Used by the alert-rule creation skill where the LLM probes
//     schemas across multiple datasources in one conversation.
//  2. session params — injected by the router when the chat is opened from
//     a datasource-scoped page (explorer, datasource query page). The
//     explorer flow doesn't require the LLM to know the id.
//  3. DB lookup by id — if args supplied id but no type, fetch plugin_type
//     from models.GetDatasourceInfosByIds. Keeps the LLM from having to
//     remember which cate each id belongs to.
//
// Returns a pre-formatted error if neither source yields a usable id, so
// callers can propagate it straight to the tool Observation.
func resolveSQLDatasource(deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (int64, string, error) {
	dsId := getArgInt64(args, "datasource_id")
	if dsId == 0 {
		dsId = getDatasourceId(params)
	}
	if dsId == 0 {
		return 0, "", fmt.Errorf("datasource_id required: pass it as a tool argument, or open the chat from a datasource-scoped page")
	}

	dsType := getArgString(args, "datasource_type")
	if dsType == "" {
		dsType = getDatasourceType(params)
	}
	if dsType == "" {
		// DB fallback — lets the LLM pass only datasource_id without
		// having to know whether id=5 is mysql or doris.
		if deps != nil && deps.DBCtx != nil {
			infos, err := models.GetDatasourceInfosByIds(deps.DBCtx, []int64{dsId})
			if err != nil {
				return 0, "", fmt.Errorf("failed to resolve datasource type for id=%d: %v", dsId, err)
			}
			if len(infos) > 0 {
				dsType = infos[0].PluginType
			}
		}
	}
	if dsType == "" {
		return 0, "", fmt.Errorf("datasource_type not resolvable for id=%d: pass datasource_type explicitly or verify the datasource exists", dsId)
	}
	return dsId, dsType, nil
}

func listDatabasesTool(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	dsId, dsType, err := resolveSQLDatasource(deps, args, params)
	if err != nil {
		return "", err
	}

	plug, exists := deps.GetSQLDatasource(dsType, dsId)
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
	case "tdengine":
		// TDengine 3.x exposes databases via information_schema. SHOW
		// DATABASES would also work but the result column set is larger
		// and the row ordering is cluttered with system tables.
		sql = "SELECT name FROM information_schema.ins_databases"
	default:
		return "", fmt.Errorf("unsupported datasource type for list_databases: %s", dsType)
	}

	// TDengine routes SQL through a single `query` field (vs `sql` for
	// the MySQL/Doris/CK/PG plugins). Normalise the shape here.
	var query map[string]interface{}
	if dsType == "tdengine" {
		query = map[string]interface{}{"query": sql}
	} else {
		query = map[string]interface{}{"sql": sql}
	}
	data, _, err := plug.QueryLog(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to list databases: %v", err)
	}

	databases := extractColumnValues(data, "database")
	logger.Debugf("list_databases: dsType=%s, found %d databases", dsType, len(databases))

	bytes, _ := json.Marshal(databases)
	return string(bytes), nil
}

func listTablesTool(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	dsId, dsType, err := resolveSQLDatasource(deps, args, params)
	if err != nil {
		return "", err
	}

	database, ok := args["database"].(string)
	if !ok || database == "" {
		return "", fmt.Errorf("database parameter is required")
	}
	if !isValidIdentifier(database) {
		return "", fmt.Errorf("invalid database name: %s", database)
	}

	plug, exists := deps.GetSQLDatasource(dsType, dsId)
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
	case "tdengine":
		// TDengine 3.x: query both regular tables and supertables so
		// alerts can be built on either. stable_name is returned by
		// ins_stables; table_name by ins_tables.
		sql = fmt.Sprintf(
			"SELECT table_name FROM information_schema.ins_tables WHERE db_name='%s' UNION ALL "+
				"SELECT stable_name AS table_name FROM information_schema.ins_stables WHERE db_name='%s'",
			database, database)
	default:
		return "", fmt.Errorf("unsupported datasource type for list_tables: %s", dsType)
	}

	var query map[string]interface{}
	if dsType == "tdengine" {
		query = map[string]interface{}{"query": sql}
	} else {
		query = map[string]interface{}{"sql": sql, "database": database}
	}
	data, _, err := plug.QueryLog(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to list tables: %v", err)
	}

	tables := extractColumnValues(data, "table")
	logger.Debugf("list_tables: dsType=%s, database=%s, found %d tables", dsType, database, len(tables))

	bytes, _ := json.Marshal(tables)
	return string(bytes), nil
}

func describeTableTool(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	dsId, dsType, err := resolveSQLDatasource(deps, args, params)
	if err != nil {
		return "", err
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

	plug, exists := deps.GetSQLDatasource(dsType, dsId)
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
	case "tdengine":
		// TDengine 3.x: query ins_columns (regular tables and supertable
		// members) and ins_tags (stable tag columns) so the LLM sees
		// both numeric columns and tag dimensions. Column alias to
		// Field/Type matches the existing MySQL/Doris convention.
		sql = fmt.Sprintf(
			"SELECT col_name AS `Field`, col_type AS `Type` FROM information_schema.ins_columns "+
				"WHERE db_name='%s' AND table_name='%s' UNION ALL "+
				"SELECT tag_name AS `Field`, tag_type AS `Type` FROM information_schema.ins_tags "+
				"WHERE db_name='%s' AND stable_name='%s'",
			database, table, database, table)
	default:
		return "", fmt.Errorf("unsupported datasource type for describe_table: %s", dsType)
	}

	var query map[string]interface{}
	if dsType == "tdengine" {
		query = map[string]interface{}{"query": sql}
	} else {
		query = map[string]interface{}{"sql": sql, "database": database}
	}
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
	// Exact key matches first (keeps deterministic order when multiple
	// columns are present, e.g. information_schema views).
	possible := map[string][]string{
		"database": {"Database", "database", "datname", "name"},
		"table":    {"table", "tablename", "table_name", "Name", "name"},
	}
	keys := possible[columnType]

	result := make([]string, 0)
	for _, row := range data {
		rowMap, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		var value string
		for _, key := range keys {
			if v, ok := rowMap[key]; ok {
				if s, ok := v.(string); ok {
					value = s
					break
				}
			}
		}
		// MySQL/Doris SHOW TABLES returns a column named Tables_in_<dbname>
		// (e.g. "Tables_in_flashcat_apm") — we can't know the exact suffix
		// ahead of time, so fall back to a prefix scan when the exact-key
		// pass didn't find anything.
		if value == "" && columnType == "table" {
			for key, v := range rowMap {
				if strings.HasPrefix(key, "Tables_in_") {
					if s, ok := v.(string); ok {
						value = s
						break
					}
				}
			}
		}
		if value != "" {
			result = append(result, value)
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
