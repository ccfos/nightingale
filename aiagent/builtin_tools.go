package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/toolkits/pkg/logger"
)

// =============================================================================
// 数据源获取函数（支持注入，便于测试）
// =============================================================================

type PromClientGetter func(dsId int64) prom.API
type SQLDatasourceGetter func(dsType string, dsId int64) (datasource.Datasource, bool)

var (
	getPromClientFunc    PromClientGetter    = defaultGetPromClient
	getSQLDatasourceFunc SQLDatasourceGetter = defaultGetSQLDatasource
)

func SetPromClientGetter(getter PromClientGetter)       { getPromClientFunc = getter }
func SetSQLDatasourceGetter(getter SQLDatasourceGetter)  { getSQLDatasourceFunc = getter }

func ResetDatasourceGetters() {
	getPromClientFunc = defaultGetPromClient
	getSQLDatasourceFunc = defaultGetSQLDatasource
}

func defaultGetPromClient(dsId int64) prom.API { return nil }

func defaultGetSQLDatasource(dsType string, dsId int64) (datasource.Datasource, bool) {
	return dscache.DsCache.Get(dsType, dsId)
}

// =============================================================================
// 内置工具注册表
// =============================================================================

// BuiltinTool 内置工具定义
type BuiltinTool struct {
	Definition AgentTool
	Handler    BuiltinToolFunc
}

var builtinTools = map[string]*BuiltinTool{
	"list_metrics": {
		Definition: AgentTool{
			Name:        "list_metrics",
			Description: "搜索 Prometheus 数据源的指标名称，支持关键词模糊匹配",
			Type:        ToolTypeBuiltin,
			Parameters: []ToolParameter{
				{Name: "keyword", Type: "string", Description: "搜索关键词，模糊匹配指标名", Required: false},
				{Name: "limit", Type: "integer", Description: "返回数量限制，默认30", Required: false},
			},
		},
		Handler: listMetrics,
	},
	"get_metric_labels": {
		Definition: AgentTool{
			Name:        "get_metric_labels",
			Description: "获取 Prometheus 指标的所有标签键及其可选值",
			Type:        ToolTypeBuiltin,
			Parameters: []ToolParameter{
				{Name: "metric", Type: "string", Description: "指标名称", Required: true},
			},
		},
		Handler: getMetricLabels,
	},
	"list_databases": {
		Definition: AgentTool{
			Name:        "list_databases",
			Description: "列出 SQL 数据源（MySQL/Doris/ClickHouse/PostgreSQL）中的所有数据库",
			Type:        ToolTypeBuiltin,
			Parameters:  []ToolParameter{},
		},
		Handler: listDatabases,
	},
	"list_tables": {
		Definition: AgentTool{
			Name:        "list_tables",
			Description: "列出指定数据库中的所有表",
			Type:        ToolTypeBuiltin,
			Parameters: []ToolParameter{
				{Name: "database", Type: "string", Description: "数据库名", Required: true},
			},
		},
		Handler: listTables,
	},
	"describe_table": {
		Definition: AgentTool{
			Name:        "describe_table",
			Description: "获取表的字段结构（字段名、类型、注释）",
			Type:        ToolTypeBuiltin,
			Parameters: []ToolParameter{
				{Name: "database", Type: "string", Description: "数据库名", Required: true},
				{Name: "table", Type: "string", Description: "表名", Required: true},
			},
		},
		Handler: describeTable,
	},
}

// GetBuiltinToolDef 获取内置工具定义
func GetBuiltinToolDef(name string) (AgentTool, bool) {
	if tool, ok := builtinTools[name]; ok {
		return tool.Definition, true
	}
	return AgentTool{}, false
}

// GetBuiltinToolDefs 获取指定的内置工具定义列表
func GetBuiltinToolDefs(names []string) []AgentTool {
	var defs []AgentTool
	for _, name := range names {
		if def, ok := GetBuiltinToolDef(name); ok {
			defs = append(defs, def)
		}
	}
	return defs
}

// GetAllBuiltinToolDefs 获取所有内置工具定义
func GetAllBuiltinToolDefs() []AgentTool {
	defs := make([]AgentTool, 0, len(builtinTools))
	for _, tool := range builtinTools {
		defs = append(defs, tool.Definition)
	}
	return defs
}

// ExecuteBuiltinTool 执行内置工具
// 返回值：result, handled, error
func ExecuteBuiltinTool(ctx context.Context, name string, params map[string]string, argsJSON string) (string, bool, error) {
	tool, exists := builtinTools[name]
	if !exists {
		return "", false, nil
	}

	var args map[string]interface{}
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			args = map[string]interface{}{"input": argsJSON}
		}
	}
	if args == nil {
		args = make(map[string]interface{})
	}

	result, err := tool.Handler(ctx, args, params)
	return result, true, err
}

// =============================================================================
// 参数提取辅助函数
// =============================================================================

func getDatasourceId(params map[string]string) int64 {
	if params == nil {
		return 0
	}
	var dsId int64
	if dsIdStr, ok := params["datasource_id"]; ok {
		fmt.Sscanf(dsIdStr, "%d", &dsId)
	}
	return dsId
}

func getDatasourceType(params map[string]string) string {
	if params == nil {
		return ""
	}
	return params["datasource_type"]
}

// =============================================================================
// Prometheus 工具实现
// =============================================================================

func listMetrics(ctx context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	dsId := getDatasourceId(params)
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id not found in params")
	}

	keyword, _ := args["keyword"].(string)
	limit := 30
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	client := getPromClientFunc(dsId)
	if client == nil {
		return "", fmt.Errorf("prometheus datasource not found: %d", dsId)
	}

	values, _, err := client.LabelValues(ctx, "__name__", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get metrics: %v", err)
	}

	result := make([]string, 0)
	keyword = strings.ToLower(keyword)
	for _, v := range values {
		m := string(v)
		if keyword == "" || strings.Contains(strings.ToLower(m), keyword) {
			result = append(result, m)
			if len(result) >= limit {
				break
			}
		}
	}

	logger.Debugf("list_metrics: found %d metrics (keyword=%s, limit=%d)", len(result), keyword, limit)

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}

func getMetricLabels(ctx context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	dsId := getDatasourceId(params)
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id not found in params")
	}

	metric, ok := args["metric"].(string)
	if !ok || metric == "" {
		return "", fmt.Errorf("metric parameter is required")
	}

	client := getPromClientFunc(dsId)
	if client == nil {
		return "", fmt.Errorf("prometheus datasource not found: %d", dsId)
	}

	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Hour)
	series, _, err := client.Series(ctx, []string{metric}, startTime, endTime)
	if err != nil {
		return "", fmt.Errorf("failed to get metric series: %v", err)
	}

	labels := make(map[string][]string)
	seen := make(map[string]map[string]bool)

	for _, s := range series {
		for k, v := range s {
			key := string(k)
			val := string(v)
			if key == "__name__" {
				continue
			}
			if seen[key] == nil {
				seen[key] = make(map[string]bool)
			}
			if !seen[key][val] {
				seen[key][val] = true
				labels[key] = append(labels[key], val)
			}
		}
	}

	logger.Debugf("get_metric_labels: metric=%s, found %d labels", metric, len(labels))

	bytes, _ := json.Marshal(labels)
	return string(bytes), nil
}

// =============================================================================
// SQL 数据源工具实现
// =============================================================================

type SQLMetadataQuerier interface {
	ListDatabases(ctx context.Context) ([]string, error)
	ListTables(ctx context.Context, database string) ([]string, error)
	DescribeTable(ctx context.Context, database, table string) ([]map[string]interface{}, error)
}

// isValidIdentifier 校验 SQL 标识符，拒绝注入关键字符
// 允许字母、数字、下划线、点号、连字符等合法字符（如 order-events）
func isValidIdentifier(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if c == ';' || c == '\'' || c == '"' || c == '`' || c == '\\' || c == 0 {
			return false
		}
	}
	return !strings.ContainsAny(s, "/*")
}

func listDatabases(ctx context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	dsId := getDatasourceId(params)
	dsType := getDatasourceType(params)
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id not found in params")
	}
	if dsType == "" {
		return "", fmt.Errorf("datasource_type not found in params")
	}

	plug, exists := getSQLDatasourceFunc(dsType, dsId)
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

	databases := extractColumnValues(data, dsType, "database")
	logger.Debugf("list_databases: dsType=%s, found %d databases", dsType, len(databases))

	bytes, _ := json.Marshal(databases)
	return string(bytes), nil
}

func listTables(ctx context.Context, args map[string]interface{}, params map[string]string) (string, error) {
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

	plug, exists := getSQLDatasourceFunc(dsType, dsId)
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

	tables := extractColumnValues(data, dsType, "table")
	logger.Debugf("list_tables: dsType=%s, database=%s, found %d tables", dsType, database, len(tables))

	bytes, _ := json.Marshal(tables)
	return string(bytes), nil
}

func describeTable(ctx context.Context, args map[string]interface{}, params map[string]string) (string, error) {
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

	plug, exists := getSQLDatasourceFunc(dsType, dsId)
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

	columns := convertToColumnInfo(data, dsType)
	logger.Debugf("describe_table: dsType=%s, table=%s.%s, found %d columns", dsType, database, table, len(columns))

	bytes, _ := json.Marshal(columns)
	return string(bytes), nil
}

// =============================================================================
// 辅助函数
// =============================================================================

type ColumnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment,omitempty"`
}

func extractColumnValues(data []interface{}, dsType string, columnType string) []string {
	result := make([]string, 0)
	for _, row := range data {
		if rowMap, ok := row.(map[string]interface{}); ok {
			var value string
			for _, key := range getPossibleColumnNames(dsType, columnType) {
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

func getPossibleColumnNames(dsType string, columnType string) []string {
	switch columnType {
	case "database":
		return []string{"Database", "database", "datname", "name"}
	case "table":
		return []string{"Tables_in_", "table", "tablename", "name", "Name"}
	default:
		return []string{}
	}
}

func convertToColumnInfo(data []interface{}, dsType string) []ColumnInfo {
	result := make([]ColumnInfo, 0)
	for _, row := range data {
		if rowMap, ok := row.(map[string]interface{}); ok {
			col := ColumnInfo{}

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
