package airunner

import (
	"sync"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/storage"
)

// SetupOptions 由宿主进程在启动期通过 Setup 一次性注入。所有字段允许零值：
// 缺失的能力对应的内置工具在被 AI 调用时返回友好错误，不会 panic。
type SetupOptions struct {
	DBCtx                  *ctx.Context
	Redis                  storage.Redis
	SkillsPath             string
	GetPromClient          func(dsId int64) prom.API
	GetSQLDatasource       func(dsType string, dsId int64) (datasource.Datasource, bool)
	FilterDatasources      func([]*models.Datasource, *models.User) []*models.Datasource
	GetAlertEvalLogs       func(ruleId string) ([]string, string, error)
	GetEventProcessingLogs func(eventHash string) ([]string, string, error)
}

var (
	runtimeMu     sync.RWMutex
	runtimeDeps   *aiagent.ToolDeps
	runtimeSkills string
)

// Setup 必须在进程接入流量之前调用，否则后续 Process 会以
// "runtime not initialized" 失败。AI Runner 设计为 center-only：edge 上
// pipeline 若配了 ai_runner 节点会一致地返回上述错误。
func Setup(opts SetupOptions) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	runtimeDeps = &aiagent.ToolDeps{
		DBCtx:                  opts.DBCtx,
		SkillsPath:             opts.SkillsPath,
		Redis:                  opts.Redis,
		GetPromClient:          opts.GetPromClient,
		GetSQLDatasource:       opts.GetSQLDatasource,
		FilterDatasources:      opts.FilterDatasources,
		GetAlertEvalLogs:       opts.GetAlertEvalLogs,
		GetEventProcessingLogs: opts.GetEventProcessingLogs,
	}
	runtimeSkills = opts.SkillsPath
}

// GetRuntime 返回 Setup 注入的依赖；未注入时 deps 为 nil，Process 会拒绝运行。
func GetRuntime() (*aiagent.ToolDeps, string) {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	return runtimeDeps, runtimeSkills
}
