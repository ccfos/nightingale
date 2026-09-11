package types

import (
	"context"
	"testing"
)

// 强只读校验默认关闭：无 CallContext、或有但未置位，都不得触发。
// 这是本次改动的核心不变量——登录用户与规则调度器的既有查询行为不能被收紧。
func TestReadOnlyEnforcedDefaultsOff(t *testing.T) {
	if ReadOnlyEnforced(context.Background()) {
		t.Error("bare context must not enforce read-only")
	}

	// 规则调度器等内部调用方只填 Operator/RuleID，不应被误判
	ctx := WithCallContext(context.Background(), CallContext{
		DatasourceID: 3,
		Operator:     "alert_rule",
		RuleID:       17,
	})
	if ReadOnlyEnforced(ctx) {
		t.Error("call context without EnforceReadOnly must not enforce read-only")
	}

	// 人工查询（登录用户）同样不置位
	ctx = WithCallContext(context.Background(), CallContext{DatasourceID: 3, Operator: "alice"})
	if ReadOnlyEnforced(ctx) {
		t.Error("human query context must not enforce read-only")
	}

	// 只有显式置位才生效——匿名分享 token 通道
	ctx = WithCallContext(context.Background(), CallContext{DatasourceID: 3, EnforceReadOnly: true})
	if !ReadOnlyEnforced(ctx) {
		t.Error("EnforceReadOnly=true must enforce read-only")
	}
}
