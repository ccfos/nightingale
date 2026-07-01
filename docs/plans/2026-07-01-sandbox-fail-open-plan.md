# 实现计划：Sandbox fail-open + 最大隔离梯子

- 对应 spec：`docs/specs/2026-07-01-sandbox-fail-open-design.md`
- 分支：`feat-sandbox-fail-open`
- 平台注意：`pkg/sandbox` 中 `sandbox.go` / `engine_unsafe.go` / `probe.go` / `config.go` / `engine.go` 跨平台编译；`engine_bwrap_linux.go` / `engine_confined_linux.go` / `cgroup_linux.go` / `probe_linux.go` 带 `//go:build linux`。本机是 darwin，涉及 linux 场景的单测用 mock caps + 测试引擎，必要时 `GOOS=linux go build/test` 交叉验证。

## 任务清单（按顺序）

### T1 — 配置字段 `RequireIsolation`
- 改 `pkg/sandbox/config.go`：`Config` 增 `RequireIsolation bool`；收窄 `DevMode` 注释（不再是 unsafe 降级门）。
- 验证：`go build ./pkg/sandbox/...` 通过；`PreCheck()` / 默认值逻辑不受影响（零值 false = fail-open）。

### T2 — Tier 增 `TierUnsafe`
- 改 `pkg/sandbox/engine.go`：新增 `TierUnsafe`（置于 `TierStrong` 之后，避免改动既有 iota 值）；`Tier.String()` 增 `case TierUnsafe: return "unsafe"`。
- 验证：单测 `TierUnsafe.String() == "unsafe"`；既有 tier 值不变（`TierDisabled==0` 等）。

### T3 — 强度顺序 + 引擎→tier 映射
- 改 `pkg/sandbox/engine.go`（或 `probe.go`）：定义 `strengthOrder = []string{EngineRunsc, EngineBwrap, EngineConfined, EngineUnsafe}`；`tierForEngine(name string) Tier`（bwrap→TierBubblewrap、confined→TierConfined、runsc→TierStrong、unsafe→TierUnsafe、其它→TierDisabled）。
- 验证：单测 `tierForEngine` 全分支。

### T4 — bwrap 工厂加 userns 门
- 改 `pkg/sandbox/engine_bwrap_linux.go`：工厂开头在 rootfs 检查前后加 `if !caps.UserNS { return nil, fmt.Errorf("unprivileged userns unavailable") }`；确认工厂无 cgroup v2 硬依赖（现状即无）。
- 验证：`GOOS=linux go build ./pkg/sandbox/...`；单测：caps 无 UserNS 时工厂返回 error，有 UserNS+bwrap+rootfs 时成功。

### T5 — 重写 `resolveEngine`（核心）
- 改 `pkg/sandbox/sandbox.go`：实现 spec §4.1 梯子——
  - `Disabled` → disable。
  - 候选：auto=strengthOrder；显式=`[配置引擎, unsafe]`（去重；未知引擎名 → disable）。
  - 逐个 `lookupEngine`+`factory`；到达 unsafe 层时 `RequireIsolation` → disable（带 lastReason），否则选 unsafe + loud warn。
  - 显式引擎失败降级到 unsafe（除非 RequireIsolation），日志 `requested %q unavailable → unsafe-exec: %v`。
  - 选中后 `s.tier = tierForEngine(engine.Name())`。
- 验证：见 T8 表驱动单测全绿。

### T6 — `logStartup` 三态文案
- 改 `pkg/sandbox/sandbox.go`：isolation 引擎→INFO ready（现状）；unsafe→醒目 WARNING（含"装 bwrap+rootfs / 设 RequireIsolation"指引 + lastReason）；disabled→WARNING（原因覆盖 RequireIsolation 场景）。
- 验证：单测捕获 `DisabledReason()` 文案；人工看启动日志分支。

### T7 — 运行结果回带隔离级别
- 改 `aiagent/tools/skill_exec.go`：`run_skill_script` 返回给模型的结果里加 `engine` / `isolation` 字段（源自 `ExecResult.Engine` / `Sandbox.EngineName()`）。
- 验证：`go build ./aiagent/...`；单测或对话中可见 "isolation: none (unsafe-exec)" / "bubblewrap"。

### T8 — resolveEngine 表驱动单测
- 改 `pkg/sandbox/sandbox_test.go`（及所需 test 辅助/测试引擎注册）：覆盖 spec §9 全 8 例。
- 验证：`go test ./pkg/sandbox/...`（本机）+ `GOOS=linux go test ./pkg/sandbox/...`（如测试依赖 linux 引擎）全绿。

### T9 — 更新既有断言 + 全量构建测试
- 更新原先断言"DevMode 才能 unsafe"的既有用例。
- 验证：`go build ./...`；`go test ./pkg/sandbox/...`；`go vet ./pkg/sandbox/...`。

## 完成定义（DoD）
- `go build ./...` 通过。
- `pkg/sandbox` 单测覆盖 spec §9 全部 8 例并通过。
- 在 darwin（非 linux）下 `resolveEngine` 走 fail-open → 选 unsafe（RequireIsolation=false）/ disabled（=true）。
- 变更集只落在 spec §10 清单内的文件。
