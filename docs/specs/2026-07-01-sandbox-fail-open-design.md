# Sandbox 降级策略：fail-open + 最大隔离梯子

- 日期：2026-07-01
- 范围：`pkg/sandbox`（引擎选择与降级策略）；波及 `aiagent` 对 `run_skill_script` 工具的挂载判定与运行结果展示
- 状态：设计已批准，待实现

## 1. 背景与动机

Skill 脚本执行由 `pkg/sandbox` 的控制面驱动：启动时探测宿主能力，选出一个隔离引擎（bubblewrap / container-confined / unsafe-exec），`Sandbox.Enabled()` 为真时 `aiagent/agent.go` 才把 `run_skill_script` 工具挂给模型。

当前实现是 **fail-closed**：

- `selectTier()`（`probe.go`）对 bubblewrap 档硬性要求 `UserNS && Seccomp && CgroupV2Delegated`，对 confined 档要求 `Seccomp && Landlock && ContainerAsBoundary`；都不满足即 `TierDisabled`。
- `resolveEngine()`（`sandbox.go`）在无隔离档时，只有 `DevMode=true` 才会退到 unsafe-exec，否则 `disable()`，`Enabled()=false`，工具不挂出。

真实后果（本次排障的现场，机器 `10.99.1.107`，内核 4.18 / RHEL 8.5）：`UserNS✓ Seccomp✓`，但 `CgroupV2✗`（cgroup v1）、`Landlock✗`（需 5.13+）、`bwrap✗`（未安装）→ tier=disabled → `run_skill_script` 不挂出 → 模型如实报告"没有 run_skill_script 工具"。而实际上该机装上 bwrap + rootfs 后**完全能做到 bubblewrap 真隔离**（cgroup v2 只影响资源配额，`setupCgroup()` 在 v1 上是优雅降级）。

目标：**让脚本执行能力在任何环境默认可用（fail-open），并在每个环境自动榨取其能提供的最强隔离；安全敏感部署可用一个开关恢复 fail-closed。**

## 2. 目标与非目标

### 目标

1. 默认 fail-open：无任何隔离能力的宿主也能执行脚本（降级到 unsafe-exec 直接执行，仍带控制面的清洁环境 / 进程组超时杀 / 输出截断）。
2. 最大隔离梯子：`Engine=auto` 时按 强→弱 逐个试真实可行性，选第一个能建成功的引擎；能力完整的环境自动拿到更强隔离。
3. cgroup v2 从 bubblewrap 的**硬门槛**降为**可选增强**（有则限额、无则不限额）。
4. 安全上限开关：新增 `RequireIsolation`（默认 `false`），为真时若唯一可用引擎是 unsafe-exec 则拒绝执行。
5. 降级对操作者与用户可见：启动日志醒目告警；`run_skill_script` 运行结果回带实际引擎/隔离级别。

### 非目标

- 不实现 runsc/gvisor、nsjail 引擎（保持"配置可识别但未编译"，梯子自动跳过）。
- 不生产/分发 bwrap 二进制或 python-base rootfs（属运维/构建路径，与本策略正交）。
- 不改动 egress 代理与 Skill Gateway 的挂载逻辑（它们已按 `EngineCaps()` 自适应，unsafe 引擎 caps 全 false 时自动 no-op）。
- 不改动 `Disabled=true` 的硬关语义。

## 3. 配置面变更（`pkg/sandbox/config.go`）

`Config` 新增字段：

```go
// RequireIsolation, when true, refuses to run skill scripts if the only
// feasible engine is unsafe-exec (no isolation). It is the safety ceiling:
// it overrides everything, including an explicit Engine="unsafe". Default
// false → fail-open (unsafe-exec is the universal floor).
RequireIsolation bool
```

不变字段：

- `Disabled bool`：硬关，最高优先级，直接 disable。
- `Engine string`：`auto`（默认）| `bubblewrap` | `confined` | `unsafe` | `runsc` | `nsjail`。显式覆盖梯子。
- `DevMode bool`：语义收窄——**不再是 unsafe 降级的门**（降级现在默认允许）。仅保留给"显式 `Engine="unsafe"` 的额外确认"等 dev 便利；见 §4 决策表。

优先级（从高到低）：`Disabled` > `RequireIsolation` > `Engine` 显式 > auto 梯子。

## 4. 引擎选择（`resolveEngine`，`pkg/sandbox/sandbox.go`）

### 4.1 算法

用一个 **强→弱的候选列表 + 逐个可行性试建** 取代原来的"tier→单引擎 + DevMode 兜底"：

```
strengthOrder = [EngineRunsc, EngineBwrap, EngineConfined, EngineUnsafe]   // 强 → 弱

resolveEngine():
    if cfg.Disabled:                      disable("sandbox.disabled=true"); return

    candidates =
        if Engine == "" or "auto":  strengthOrder
        else:                       [ configEngineToName(Engine) ]   // 未知名 → disable
                                    然后追加 EngineUnsafe 作为降级兜底（去重）

    for name in candidates:
        if name == EngineUnsafe:
            # 到达兜底层
            if cfg.RequireIsolation:
                disable("RequireIsolation=true 且无隔离能力引擎可用：" + lastReason); return
            build unsafe; select; loud-warn; return
        if not lookupEngine(name):        record "not compiled in"; continue
        eng, err = factory(name)(cfg, caps)   # 见 §4.2 各引擎门
        if err != nil:                    lastReason = err; warn; continue
        select eng; return
```

要点：

- **显式引擎失败也会降级到 unsafe**（除非 `RequireIsolation`），日志写明 `requested %q unavailable → unsafe-exec: %v`。这保证"总能跑"，同时把降级说清楚。
- **`RequireIsolation=true` 压过显式 `Engine="unsafe"`**：候选里到达 unsafe 层即 disable。这是安全上限，语义单一好记。
- 到达 unsafe 兜底时携带 `lastReason`（上一个隔离引擎为何建不起来），让 disable / warn 的原因可操作。

### 4.2 各引擎可行性门（工厂内自检，返回 error 即降级）

| 引擎 | 门槛 | cgroup v2 |
|---|---|---|
| `runsc` | 编译进来 + `runsc` 二进制在 PATH（当前未编译 → 恒跳过） | — |
| `bubblewrap` | `caps.BwrapPath != ""`（宿主或内嵌） + rootfs base 可用（`Rootfs.Path` 或内嵌） + `caps.UserNS` | **移出硬门槛**，运行时 `setupCgroup` 有则限额、无则空 handle |
| `confined` | `caps.Seccomp && caps.Landlock && cfg.ContainerAsBoundary` | — |
| `unsafe` | 恒可建（无宿主依赖） | — |

具体改动：

- bubblewrap 工厂（`engine_bwrap_linux.go`）新增 `caps.UserNS` 检查：缺失时返回 `error("unprivileged userns unavailable")`，使梯子降级而非在 Run 时反复失败。已知限制：本阶段不探测 setuid-bwrap（legacy），userns 关闭的 setuid-bwrap 宿主会被判不可用并降到下一档；后续可加探测。
- `selectTier()` 不再决定"能不能用"；cgroup v2 只作为诊断信息与 tier 上报输入。bubblewrap 是否可用完全交给工厂门。

### 4.3 决策表（Engine=auto，RequireIsolation=false）

| UserNS | Seccomp | CgroupV2 | Landlock | bwrap+rootfs | 选中引擎 |
|:-:|:-:|:-:|:-:|:-:|---|
| ✓ | ✓ | ✓ | any | ✓ | bubblewrap（含资源配额） |
| ✓ | ✓ | ✗ | any | ✓ | **bubblewrap（无资源配额）← .107 目标态** |
| ✗ | ✓ | any | ✓（且 ContainerAsBoundary） | any | confined |
| any | any | any | any | ✗ | unsafe-exec（fail-open，大声告警） |
| 上一行同条件，但 RequireIsolation=true | | | | | disabled（拒绝，给可操作原因） |

## 5. Tier 上报（`pkg/sandbox/engine.go`）

- 新增枚举 `TierUnsafe`（最弱、非 disabled），`Tier.String()` 增加 `"unsafe"` 分支。
- 选中引擎后按引擎名回填 `s.tier`：`bubblewrap→TierBubblewrap`、`confined→TierConfined`、`runsc→TierStrong`、`unsafe→TierUnsafe`、无→`TierDisabled`。让 `Tier()` / admin 展示反映**实际**隔离级别而非"理想档"。
- `selectTier()` 保留为诊断/日志用途（"此宿主理想档 = X，因缺 Y 未达成"），不再作为可用性判据。

## 6. 启动日志与 `Enabled()`（`sandbox.go`）

- `Enabled()` 逻辑不变（`engine != nil`）。因 unsafe 兜底，绝大多数环境为真 → `run_skill_script` 基本处处挂出（达成目标）。`RequireIsolation=true` 且仅 unsafe 可用时 `engine=nil` → 该操作者维持 fail-closed，工具不挂出。
- `logStartup()` 三态：
  - 隔离引擎（bubblewrap/confined/runsc）：`INFO sandbox: ready engine=... tier=... caps=...`（保持现状）。
  - unsafe 引擎：`WARNING sandbox: SKILL EXECUTION RUNNING WITHOUT ISOLATION (unsafe-exec) — install bubblewrap+rootfs for real isolation, or set RequireIsolation=true to refuse. reason=<上一个隔离引擎为何不可用>`。
  - disabled：`WARNING sandbox: SKILL EXECUTION DISABLED — <可操作原因> (tier=..., os=..., kernel=...)`（保持现状，原因文案覆盖 RequireIsolation 场景）。

## 7. 每次运行可见性

- unsafe 引擎 `Run()` 已按每次执行打 WARNING；`ExecResult.Engine` 已由 `aiagent/skill_runtime/executor.go:144` 带入运行结果。
- 增强：`run_skill_script` 工具（`aiagent/tools/skill_exec.go`）在返回给模型的结果中带上 `engine` / 隔离级别字段（如 `isolation: "none (unsafe-exec)"` vs `"bubblewrap"`），使降级在对话中对用户可见，而非仅埋在服务端日志。

## 8. 影响面核查（下游不破坏）

- `aiagent/agent.go:129`：仍按 `Enabled()` 挂 `run_skill_script`——目标即是让它几乎处处为真。
- `aiagent/tools/skill_exec.go` / `skill_runtime/executor.go`：读 `Enabled()` + `DisabledReason()`，语义不变；RequireIsolation 场景复用 `DisabledReason()` 文案。
- `aiagent/skill_runtime/control.go:40,53`：读 `EngineCaps().Network / .Namespaces` 决定 egress 代理与 Gateway 挂载。unsafe 引擎 `Caps()` 全 false → 自动 no-op（无网络代理、无绑定挂载），符合预期，不崩。

## 9. 测试（`pkg/sandbox/sandbox_test.go` 等）

表驱动覆盖 `resolveEngine` 梯子（用 mock caps + 可控工厂）：

1. 全能力 + bwrap + rootfs → bubblewrap。
2. UserNS+Seccomp，无 CgroupV2，bwrap + rootfs → bubblewrap（cgroup 放宽，**.107 场景**）。
3. 无 bwrap/rootfs → unsafe（fail-open）。
4. 无 bwrap/rootfs + `RequireIsolation=true` → disabled，`DisabledReason()` 非空且可操作。
5. 显式 `Engine="bubblewrap"` 但缺 rootfs → unsafe（降级）；叠加 `RequireIsolation=true` → disabled。
6. 显式 `Engine="unsafe"` → unsafe；叠加 `RequireIsolation=true` → disabled（安全上限压过显式）。
7. bwrap 存在但 `UserNS=false` → 跳过 bubblewrap，降到下一档/unsafe。
8. `Disabled=true` → disabled（不受 fail-open 影响）。

同时更新/替换原先断言"DevMode 才能 unsafe"的既有用例（如 `sandbox_test.go` 中 unsafe 相关断言），使其符合新默认。

## 10. 变更文件清单

- `pkg/sandbox/config.go`：新增 `RequireIsolation`；`DevMode` 注释收窄。
- `pkg/sandbox/sandbox.go`：重写 `resolveEngine`（强→弱梯子 + fail-open 兜底 + RequireIsolation 上限）；`logStartup` 三态文案；选中后回填 `s.tier`。
- `pkg/sandbox/engine.go`：新增 `TierUnsafe` + `String()` 分支；`strengthOrder` 常量（或就近定义）。
- `pkg/sandbox/engine_bwrap_linux.go`：工厂新增 `UserNS` 门；确认无 cgroup v2 硬依赖。
- `pkg/sandbox/probe.go`：`selectTier` 降级为诊断用途（保留，注释说明）。
- `aiagent/tools/skill_exec.go`：运行结果回带 engine/隔离级别。
- `pkg/sandbox/sandbox_test.go`（及相关 _test）：新增梯子表驱动用例，更新旧断言。
- 配置样例（`etc*/config.toml` 或文档）：补 `[Sandbox] RequireIsolation` 说明（可选，随实现附带）。

## 10.1 附:DataDir 默认值改为临时目录(fail-open 的一部分)

排障发现:fail-open 让引擎在 .107 可用了,但 `NewWorkspace()` 在 `DataDir/sessions/<execID>` 建目录时失败——默认 `DataDir=/var/lib/n9e/sandbox`(FHS 路径)在非 root 的 `flashcat` 进程下无法创建(`/var/lib` root 所有),报含糊的"permission denied",且发生在 `Run()` 之前故无审计行、无服务端日志。这与"任何环境都可用"矛盾。

改动:`config.go` 的 `PreCheck()` 把 `DataDir` 空值默认从 `/var/lib/n9e/sandbox` 改为 `filepath.Join(os.TempDir(), "n9e-sandbox")`。理由:临时目录几乎总可写(非 root 部署开箱即用);工作区本就每次执行即用即删(`Workspace.Cleanup()`),temp 生命周期正好匹配。运维仍可用显式 `[Sandbox] DataDir` 固定路径。注意:`/tmp` 若 `noexec` 不影响解释型脚本(execve 的是 `python3` 而非脚本文件),仅自带裸 ELF 的技能会踩,属边缘情况。移除了 `defaultDataDir` 常量(改用 `os.TempDir()` 运行时求值)。测试:`TestConfigPreCheckDefaults` 增断言默认值。

## 11. 迁移与兼容

- 默认行为变化：原先 fail-closed 的部署升级后将变为 fail-open（无隔离时跑 unsafe）。这是**有意的默认翻转**，通过启动 WARNING 明示。需要维持旧行为的部署设 `RequireIsolation=true`。
- 已配 `DevMode=true + Engine=unsafe` 的 dev 部署：行为不变（仍 unsafe）。
- 已能满足 bubblewrap 全门槛的生产部署：行为不变（仍 bubblewrap，含资源配额）。
