package aiagent

import "strings"

// 本文件是写类工具幂等的 agent 侧原语：单次 Run 的整个工具循环内（跨迭代）
// 去重。resume 重放的跨进程效果台账在路由层
// （center/router/router_ai_interrupt.go，Redis 实现）。

// writeToolPrefixes 标记"会产生副作用"的内置工具命名前缀（与路由层抽卡名单的
// create_*/update_*/import_* 同一命名约定）。
var writeToolPrefixes = []string{"create_", "update_", "import_", "delete_", "add_"}

// isWriteTool 按命名约定判断工具是否为写类工具。
func isWriteTool(name string) bool {
	for _, p := range writeToolPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// turnWriteDeduper 对写类工具做"同名同参只执行一次"。作用域 = 单次 Run 的
// 整个工具循环（**跨迭代**，turn 级而非 iteration 级；每个 Run 新建实例，
// 不跨对话轮）：模型在同一循环内用完全相同的参数重复调用同一个写工具，
// 几乎必然是迷惑/重试（合法的"再建一条"参数必然不同），直接复用首次结果，
// 避免重复落库。读类工具不去重（重复读无害，且轮询类工具需要重复执行）。
type turnWriteDeduper struct {
	seen map[string]string // tool+"\x00"+args → 首次执行结果
}

func newTurnWriteDeduper() *turnWriteDeduper {
	return &turnWriteDeduper{seen: map[string]string{}}
}

func (d *turnWriteDeduper) key(tool, args string) string {
	return tool + "\x00" + args
}

// lookup 返回 (首次结果, 是否命中)。仅写类工具参与。
func (d *turnWriteDeduper) lookup(tool, args string) (string, bool) {
	if !isWriteTool(tool) {
		return "", false
	}
	out, ok := d.seen[d.key(tool, args)]
	return out, ok
}

// record 记录写类工具的执行结果（含错误观测——重试同样参数会得到同样结果，
// 缓存它语义一致且能止住无效重试循环）。
func (d *turnWriteDeduper) record(tool, args, result string) {
	if !isWriteTool(tool) {
		return
	}
	d.seen[d.key(tool, args)] = result
}
