package aiagent

import (
	"context"
	"encoding/json"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/prom"
)

// =============================================================================
// Dependency injection (exported for tools sub-package)
// =============================================================================

type PromClientGetter func(dsId int64) prom.API
type SQLDatasourceGetter func(dsType string, dsId int64) (datasource.Datasource, bool)
type DatasourceFilterFunc func([]*models.Datasource, *models.User) []*models.Datasource

var (
	dbCtx                *ctx.Context
	skillsPath           string
	getPromClientFunc    PromClientGetter     = func(dsId int64) prom.API { return nil }
	getSQLDatasourceFunc SQLDatasourceGetter  = defaultGetSQLDatasource
	datasourceFilterFunc DatasourceFilterFunc = func(ds []*models.Datasource, u *models.User) []*models.Datasource { return ds }
)

func defaultGetSQLDatasource(dsType string, dsId int64) (datasource.Datasource, bool) {
	return dscache.DsCache.Get(dsType, dsId)
}

func SetDBCtx(c *ctx.Context)                              { dbCtx = c }
func SetSkillsPath(path string)                             { skillsPath = path }
func SetPromClientGetter(getter PromClientGetter)           { getPromClientFunc = getter }
func SetSQLDatasourceGetter(getter SQLDatasourceGetter)     { getSQLDatasourceFunc = getter }
func SetDatasourceFilter(filter DatasourceFilterFunc)        { datasourceFilterFunc = filter }

func ResetDatasourceGetters() {
	getPromClientFunc = func(dsId int64) prom.API { return nil }
	getSQLDatasourceFunc = defaultGetSQLDatasource
	datasourceFilterFunc = func(ds []*models.Datasource, u *models.User) []*models.Datasource { return ds }
}

// Exported getters for tools sub-package
func GetDBCtx() *ctx.Context                                              { return dbCtx }
func GetSkillsPath() string                                               { return skillsPath }
func GetPromClient(dsId int64) prom.API                                   { return getPromClientFunc(dsId) }
func GetSQLDatasource(dsType string, dsId int64) (datasource.Datasource, bool) {
	return getSQLDatasourceFunc(dsType, dsId)
}
func FilterDatasources(ds []*models.Datasource, user *models.User) []*models.Datasource {
	return datasourceFilterFunc(ds, user)
}

// =============================================================================
// Builtin tool registry
// =============================================================================

// BuiltinTool pairs a tool definition with its handler.
type BuiltinTool struct {
	Definition AgentTool
	Handler    BuiltinToolFunc
}

var builtinTools = map[string]*BuiltinTool{}

// RegisterBuiltinTool registers a builtin tool. Called by tools sub-package init().
func RegisterBuiltinTool(name string, bt *BuiltinTool) {
	builtinTools[name] = bt
}

// GetBuiltinToolDef returns a single builtin tool definition.
func GetBuiltinToolDef(name string) (AgentTool, bool) {
	if tool, ok := builtinTools[name]; ok {
		return tool.Definition, true
	}
	return AgentTool{}, false
}

// GetBuiltinToolDefs returns definitions for the given tool names.
func GetBuiltinToolDefs(names []string) []AgentTool {
	var defs []AgentTool
	for _, name := range names {
		if def, ok := GetBuiltinToolDef(name); ok {
			defs = append(defs, def)
		}
	}
	return defs
}

// GetAllBuiltinToolDefs returns all registered builtin tool definitions.
func GetAllBuiltinToolDefs() []AgentTool {
	defs := make([]AgentTool, 0, len(builtinTools))
	for _, tool := range builtinTools {
		defs = append(defs, tool.Definition)
	}
	return defs
}

// ExecuteBuiltinTool executes a builtin tool by name.
// Returns (result, handled, error). handled=false means the tool was not found.
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
