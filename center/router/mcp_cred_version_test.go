package router

import "testing"

// 凭据版本必须固定「两个 token 密文」，不能只盯 access token。
//
// 触发场景（本轮 review 的实测反例）：实例 A、B 都持有 (X, R1)。B 用 R1 刷新成功，
// provider 回了**同一个 access token X** + 轮换后的 refresh token R2，B 存下 (X, R2)。
// 随后 A 的 R1 已被消费，刷新拿到 invalid_grant。若版本只是 access token，A 的
// WHERE access_token=X 仍会命中，把 B 刚存的 R2 一起清掉 —— 整个组织的 MCP 授权
// 再次全局丢失。OAuth 并不保证 refresh 后签发不同的 access token，所以「凭据变了
// ⇒ access token 变了」这个前提是错的。
func TestCredVersionPinsBothTokens(t *testing.T) {
	// A 装配时固定 (X, R1)
	ver := &mcpCredVersion{accessCipher: "X", refreshCipher: "R1"}

	acc, ref := ver.get()
	if acc != "X" || ref != "R1" {
		t.Fatalf("get() = (%q,%q), want (X,R1)", acc, ref)
	}

	// A 失效时用 (X, R1) 做条件更新；此刻库里是 B 存下的 (X, R2)。
	// access 相同、refresh 不同 —— 条件必须整体不匹配，新凭据得以保全。
	const storedAccess, storedRefresh = "X", "R2"
	if acc == storedAccess && ref == storedRefresh {
		t.Fatal("pinned version must NOT match a credential whose refresh token was rotated; " +
			"matching here is exactly what wipes the freshly saved R2")
	}
	if acc != storedAccess {
		t.Fatal("this scenario requires the access token to be unchanged — otherwise it doesn't reproduce the bug")
	}
}

// refresh 会轮换库里的密文，版本必须跟着走 —— 否则真正该清的凭据反而条件不匹配、
// 永远清不掉（这个方向失败是安全的，但会让 DB 一直脏、按钮每轮都弹）。
func TestCredVersionFollowsRefresh(t *testing.T) {
	ver := &mcpCredVersion{accessCipher: "X", refreshCipher: "R1"}
	ver.set("Y", "R2") // persistRefreshedMCPToken 保存成功后推进版本

	acc, ref := ver.get()
	if acc != "Y" || ref != "R2" {
		t.Fatalf("get() after refresh = (%q,%q), want (Y,R2)", acc, ref)
	}
}

// nil 版本不得 panic，且取空值 —— invalidateMCPOAuthCredential 会据此直接跳过，
// 不会发出一条 WHERE 全空的更新。
func TestCredVersionNilSafe(t *testing.T) {
	var ver *mcpCredVersion
	acc, ref := ver.get()
	if acc != "" || ref != "" {
		t.Fatalf("nil get() = (%q,%q), want empty", acc, ref)
	}
	ver.set("X", "R1") // 不得 panic
}
