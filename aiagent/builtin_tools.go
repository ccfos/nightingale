package aiagent

import (
	"context"
	"encoding/json"
)

// =============================================================================
// Builtin tool registry
//
// 只保留 init-time 注册表：aiagent/tools 子包的 init() 注册条目，运行期只读。
// 宿主依赖（DBCtx、datasource 获取器等）见 ToolDeps（types.go），由 Agent 显式注入。
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
func ExecuteBuiltinTool(ctx context.Context, deps *ToolDeps, name string, params map[string]string, argsJSON string) (string, bool, error) {
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

	result, err := tool.Handler(ctx, deps, args, params)
	return result, true, err
}
