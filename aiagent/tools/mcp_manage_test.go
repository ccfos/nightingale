package tools

import (
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// 回归：token 非空但不可用（密钥轮换后解密失败 / 已过期且无 refresh）时，绝不能报
// 「已连接」。旧实现按 access_token 列非空判定，会让模型对着一台根本连不上的 server
// 告诉用户一切正常，同时前端把授权按钮藏掉——正是授权按钮要救的场景。
// 这里用注入的 MCPOAuthUsable 模拟宿主的真实解密判定。
func TestMCPOAuthConnected(t *testing.T) {
	oauthServer := &models.MCPServer{Id: 7, Name: "oauth-mcp", AuthMode: "oauth"}
	headerServer := &models.MCPServer{Id: 8, Name: "header-mcp", AuthMode: "header"}

	cases := []struct {
		name   string
		server *models.MCPServer
		usable func(int64) bool
		want   bool
	}{
		{"oauth usable", oauthServer, func(int64) bool { return true }, true},
		// token 非空但解密不出 → 宿主判定 false → 必须报未连接（本次回归点）
		{"oauth stored but undecryptable", oauthServer, func(int64) bool { return false }, false},
		// 未注入判定器（CLI/单测）时保守报未连接，不能凭空说可用
		{"oauth without checker", oauthServer, nil, false},
		// 非 oauth 模式无所谓授权态，恒 false
		{"header mode never connected", headerServer, func(int64) bool { return true }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := &aiagent.ToolDeps{MCPOAuthUsable: tc.usable}
			if got := mcpOAuthConnected(deps, tc.server); got != tc.want {
				t.Fatalf("mcpOAuthConnected(%s) = %v, want %v", tc.server.Name, got, tc.want)
			}
		})
	}
}
