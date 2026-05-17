package airunner

import (
	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/storage"
)

// SetupOptions 是宿主（alert / center）注入 ai_runner 运行期依赖的入口。
// 各字段都允许为 nil/零值：未提供的能力对应的内置工具在被 AI 调用时返回
// 友好错误，不会 panic。这样 alert/edge 这种缺管理面能力的进程也能跑
// ai_runner，只是可调用的内置工具集合是 center 的子集。
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

// Setup 构造 ToolDeps 并通过 SetRuntime 注入到 ai_runner 处理器。
//
// 重复调用：后调覆盖前调。在 center 进程下，alert.Start 会先调一次精简版，
// 之后 router 启动期再调一次完整版——调用顺序由 center 主流程保证。
func Setup(opts SetupOptions) {
	SetRuntime(&aiagent.ToolDeps{
		DBCtx:                  opts.DBCtx,
		SkillsPath:             opts.SkillsPath,
		Redis:                  opts.Redis,
		GetPromClient:          opts.GetPromClient,
		GetSQLDatasource:       opts.GetSQLDatasource,
		FilterDatasources:      opts.FilterDatasources,
		GetAlertEvalLogs:       opts.GetAlertEvalLogs,
		GetEventProcessingLogs: opts.GetEventProcessingLogs,
	}, opts.SkillsPath)
}
