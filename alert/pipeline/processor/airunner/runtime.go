package airunner

import (
	"sync"

	"github.com/ccfos/nightingale/v6/aiagent"
)

// AI Runner 处理器在 models.Processor.Init 时不持有宿主的依赖（DBCtx、
// PromClient、SkillsPath 等），但 Process 阶段又需要复用 chat 路径已经构造
// 好的那一套 ToolDeps 和 skill 目录。这里提供一个进程级的注入点：宿主
// 启动期调用 SetRuntime 把运行期依赖灌进来，处理器在执行时通过 GetRuntime
// 取出使用。
//
// 双进程注入约定：center 进程会先后调用两次 SetRuntime——alert.Start 内部
// 注入一份精简依赖（DBCtx/Redis/SkillsPath），随后 router 启动期再以完整
// 依赖（含 PromClients、AlertEvalLogs 等）覆盖。**调用方必须保证 center
// 的注入在 alert.Start 之后**，否则完整依赖会被精简依赖覆盖。

var (
	runtimeMu         sync.RWMutex
	runtimeToolDeps   *aiagent.ToolDeps
	runtimeSkillsPath string
	runtimeEnabled    bool
)

// SetRuntime 注入 AI Runner 处理器运行期使用的依赖。可重复调用，最后一次生效。
// 调用 SetRuntime 即表示宿主已显式启用 ai_runner，Process 会放行；未调用
// SetRuntime 的进程上 Process 会直接报错，避免在未显式开启时产生外部 LLM 调用。
func SetRuntime(deps *aiagent.ToolDeps, skillsPath string) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	runtimeToolDeps = deps
	runtimeSkillsPath = skillsPath
	runtimeEnabled = true
}

// GetRuntime 取出 AI Runner 运行期依赖；未注入时返回 (nil, "", false)。
func GetRuntime() (*aiagent.ToolDeps, string, bool) {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	return runtimeToolDeps, runtimeSkillsPath, runtimeEnabled
}
