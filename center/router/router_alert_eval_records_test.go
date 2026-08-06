package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/evallog"
)

// 本机扇出必须对齐引擎侧的并发闸。
//
// 合并部署下（center 与 alert 同进程，默认形态），一条命中多个数据源的规则，它的 owner
// 往往全是本机；本机扇出若大于闸值，超出的那几路排队到超时后各拿一个 ErrBusy，一次普通的
// 页面请求就变成半屏"引擎忙"，而这纯粹是自己挤自己。
func TestEvalRecordsLocalFanoutMatchesEngineGate(t *testing.T) {
	const gate = 3
	if evalRecordsFanout <= gate {
		t.Skip("远程扇出已不大于闸值，本用例要防的自伤场景不存在了")
	}

	// 引擎未启用（center 单独部署）：兜底为 1，不能是 0，否则信号量永远拿不到令牌
	evallog.Shutdown()
	if got := evalRecordsLocalFanout(); got != 1 {
		t.Fatalf("with evallog disabled, local fanout = %d, want 1", got)
	}

	if err := evallog.Init(evallog.Config{Dir: t.TempDir(), MaxConcurrentQueries: gate}, evallog.Hooks{}); err != nil {
		t.Fatalf("init evallog: %v", err)
	}
	t.Cleanup(evallog.Shutdown)

	if got := evalRecordsLocalFanout(); got != gate {
		t.Fatalf("local fanout = %d, want %d (the engine's MaxConcurrentQueries)", got, gate)
	}
}

// 边收边收口：合并结果任何时候都不该超过 limit，且必须按 ts 倒序。
func TestSortTruncEvalRecords(t *testing.T) {
	recs := []evallog.EvalRecord{{Ts: 3}, {Ts: 1}, {Ts: 5}, {Ts: 2}}

	got := sortTruncEvalRecords(recs, 2)
	if len(got) != 2 || got[0].Ts != 5 || got[1].Ts != 3 {
		t.Fatalf("expect the newest 2 in desc order, got %+v", got)
	}

	// 未超 limit 时也要保证有序（调用方直接把它当最终结果返回）
	got = sortTruncEvalRecords([]evallog.EvalRecord{{Ts: 1}, {Ts: 9}}, 10)
	if len(got) != 2 || got[0].Ts != 9 {
		t.Fatalf("expect desc order when under the limit, got %+v", got)
	}

	if got := sortTruncEvalRecords(nil, 10); len(got) != 0 {
		t.Fatalf("nil input should stay empty, got %+v", got)
	}
}
