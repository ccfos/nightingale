package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// mcpOAuthWatch 把两类需要重授权的 server 汇成一张卡片：装配前本地预检就判死的，
// 以及凭据被服务端撤销、要等 agent 真正连接才暴露的。后者是本次要补的洞——它在
// buildMCPConfigForAgent 返回之后才发生，所以卡片必须等 agent 跑完再读 watch。
func TestMCPOAuthWatch(t *testing.T) {
	a := &models.MCPServer{Id: 1, Name: "a"}
	b := &models.MCPServer{Id: 2, Name: "b"}
	c := &models.MCPServer{Id: 3, Name: "c"}

	w := newMCPOAuthWatch()
	w.track(a, true)  // 预检即判定需要授权
	w.track(b, false) // 预检通过，仅登记为候选
	w.track(c, false)

	// 运行时之前：只有预检判死的那台
	if got := w.servers(); len(got) != 1 || got[0].Id != a.Id {
		t.Fatalf("before runtime failures = %v, want only server a", ids(got))
	}

	// 运行时 b 的凭据被拒（撤销 / invalid_grant）
	if first := w.markFailed(b.Id); !first {
		t.Fatal("markFailed(b) first call should report true so the token is invalidated once")
	}
	if again := w.markFailed(b.Id); again {
		t.Fatal("markFailed(b) second call should report false — token invalidation must not repeat")
	}

	// 合并后按绑定顺序返回 a、b；c 始终没失败，不该出现
	got := w.servers()
	if len(got) != 2 || got[0].Id != a.Id || got[1].Id != b.Id {
		t.Fatalf("after runtime failure = %v, want [a b] in binding order", ids(got))
	}
}

// 预检判死的 server 也要能被 markFailed 幂等处理（同一台可能两条路径都命中）。
func TestMCPOAuthWatchPreflightThenRuntime(t *testing.T) {
	a := &models.MCPServer{Id: 1, Name: "a"}
	w := newMCPOAuthWatch()
	w.track(a, true)
	if first := w.markFailed(a.Id); first {
		t.Fatal("markFailed on an already-needed server should report false, not re-invalidate")
	}
	if got := w.servers(); len(got) != 1 {
		t.Fatalf("servers() = %v, want a listed exactly once", ids(got))
	}
}

// 未 track 的 server 不该出现在卡片里：没资格授权的用户（无 RBAC 权限或不在管理团队）
// 根本不会被 track，拿到按钮也只会 403。
func TestMCPOAuthWatchUntrackedNeverSurfaces(t *testing.T) {
	w := newMCPOAuthWatch()
	w.markFailed(99)
	if got := w.servers(); len(got) != 0 {
		t.Fatalf("servers() = %v, want empty for an untracked server", ids(got))
	}
}

func ids(lst []*models.MCPServer) []int64 {
	out := make([]int64, 0, len(lst))
	for _, s := range lst {
		out = append(out, s.Id)
	}
	return out
}
