package chat

import (
	"encoding/json"
	"math"
	"testing"
)

// private(可见范围) 是授权字段，转发时绝不能 fail-open：兜底成 0 恰好是「公开」，
// 等于替用户把资源放开给所有人，且会让 resolveCreationPrivate 误判为「已提交」而
// 不再弹表单。只认确定的整数，其余一律不转发，交回「未提交」语义。
func TestContextForwardInputsPrivate(t *testing.T) {
	cases := []struct {
		name      string
		ctx       map[string]interface{}
		want      string
		wantFwded bool
	}{
		// 确定的整数：转发（0 是合法值，必须按存在性而非真值转发）
		{"float64 public", map[string]interface{}{"private": float64(0)}, "0", true},
		{"float64 team-scoped", map[string]interface{}{"private": float64(1)}, "1", true},
		{"int", map[string]interface{}{"private": 1}, "1", true},
		{"int64", map[string]interface{}{"private": int64(0)}, "0", true},
		{"string", map[string]interface{}{"private": "1"}, "1", true},
		{"json.Number", map[string]interface{}{"private": json.Number("0")}, "0", true},

		// 非整数 / 非法类型：不转发（旧实现会全部静默变成 "0"=公开）
		{"fractional float not forwarded", map[string]interface{}{"private": 0.5}, "", false},
		{"fractional json.Number not forwarded", map[string]interface{}{"private": json.Number("0.5")}, "", false},
		{"nil not forwarded", map[string]interface{}{"private": nil}, "", false},
		{"bool not forwarded", map[string]interface{}{"private": true}, "", false},
		{"array not forwarded", map[string]interface{}{"private": []interface{}{float64(1)}}, "", false},
		{"object not forwarded", map[string]interface{}{"private": map[string]interface{}{"a": 1}}, "", false},
		{"blank string not forwarded", map[string]interface{}{"private": "  "}, "", false},
		{"absent", map[string]interface{}{}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ContextForwardInputs(&AIChatRequest{Context: tc.ctx})
			v, ok := got["private"]
			if ok != tc.wantFwded {
				t.Fatalf("ContextForwardInputs(%v) private forwarded = %v (%q), want forwarded %v", tc.ctx, ok, v, tc.wantFwded)
			}
			if ok && v != tc.want {
				t.Fatalf("ContextForwardInputs(%v) private = %q, want %q", tc.ctx, v, tc.want)
			}
		})
	}
}

// team_ids(管理团队) 同样是授权字段。旧实现经 ctxInt64Slice 把 [1.9] 直接 int64() 成
// 团队 1：篡改的 action.param 就能把资源分配给用户根本没提交的真实团队，而下游 subset
// 校验对「团队 1」是放行的（调用者确实属于团队 1），越权因此无声通过。任一元素非法必须
// 整组作废 → 回退「未提交」→ 重新弹表单。
func TestContextForwardInputsTeamIDs(t *testing.T) {
	cases := []struct {
		name      string
		ctx       map[string]interface{}
		want      string
		wantFwded bool
	}{
		{"integral floats", map[string]interface{}{"team_ids": []interface{}{float64(1), float64(2)}}, "1,2", true},
		{"json.Number", map[string]interface{}{"team_ids": []interface{}{json.Number("3")}}, "3", true},

		// 核心回归：小数绝不能被截断成一个真实团队
		{"fractional element voids the group", map[string]interface{}{"team_ids": []interface{}{1.9}}, "", false},
		{"one bad element voids the whole group", map[string]interface{}{"team_ids": []interface{}{float64(1), 2.5}}, "", false},
		{"fractional json.Number voids", map[string]interface{}{"team_ids": []interface{}{json.Number("1.9")}}, "", false},

		// 其余非法输入同样整组作废，而不是静默丢弃该元素（那会悄悄缩小提交的集合）
		{"nan voids", map[string]interface{}{"team_ids": []interface{}{math.NaN()}}, "", false},
		{"inf voids", map[string]interface{}{"team_ids": []interface{}{math.Inf(1)}}, "", false},
		{"zero voids", map[string]interface{}{"team_ids": []interface{}{float64(0)}}, "", false},
		{"negative voids", map[string]interface{}{"team_ids": []interface{}{float64(-1)}}, "", false},
		{"string element voids", map[string]interface{}{"team_ids": []interface{}{"1"}}, "", false},
		{"bool element voids", map[string]interface{}{"team_ids": []interface{}{true}}, "", false},
		{"non-array voids", map[string]interface{}{"team_ids": float64(1)}, "", false},
		{"absent", map[string]interface{}{}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ContextForwardInputs(&AIChatRequest{Context: tc.ctx})
			v, ok := got["team_ids"]
			if ok != tc.wantFwded {
				t.Fatalf("ContextForwardInputs(%v) team_ids forwarded = %v (%q), want forwarded %v", tc.ctx, ok, v, tc.wantFwded)
			}
			if ok && v != tc.want {
				t.Fatalf("ContextForwardInputs(%v) team_ids = %q, want %q", tc.ctx, v, tc.want)
			}
		})
	}
}
