package router

import (
	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/airunner"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/pkg/prom"
)

// registerAIRunnerRuntime 把 AI Runner 事件处理器运行期需要的依赖（DBCtx、
// PromClient、DatasourceFilter、SkillsPath 等）注入 airunner 包。
//
// 与 chat 路径在每次请求时都重新构造 ToolDeps 不同：处理器路径每条事件都
// 会走，配一次复用更划算；这些依赖本身也都是进程级单例。
//
// 与 alert.Start 内部注入的关系：alert.Start 会以"精简依赖"先调用一次
// airunner.Setup，本函数在 router 启动期再用"完整依赖"覆盖。调用顺序由
// center 主流程保证（先 alert.Start，再 router.New → 本函数）。
func (rt *Router) registerAIRunnerRuntime() {
	airunner.Setup(airunner.SetupOptions{
		DBCtx:      rt.Ctx,
		Redis:      rt.Redis,
		SkillsPath: rt.Center.AIAgent.SkillsPath,
		GetPromClient: func(dsId int64) prom.API {
			return rt.PromClients.GetCli(dsId)
		},
		GetSQLDatasource: func(dsType string, dsId int64) (datasource.Datasource, bool) {
			return dscache.DsCache.Get(dsType, dsId)
		},
		FilterDatasources:      rt.DatasourceCache.DatasourceFilter,
		GetAlertEvalLogs:       rt.getAlertEvalLogs,
		GetEventProcessingLogs: rt.getEventLogs,
	})
}
