package mcp

import (
	"context"
	"net/http"
	"testing"
)

// Authorize() 必须只弹按钮、绝不判死凭据。
//
// SDK 的契约是「任意 401/403 都调 Authorize」（mcp/streamable.go 按响应码直接分派，
// 中间没有任何 refresh；token 未过期时 token source 直接返回缓存值、根本不碰 token
// endpoint）。所以一个与凭据无关的 403（某工具 scope 不足、RFC 6750 insufficient_scope、
// 前置网关/WAF、IP 白名单）也会走到这里。若据此清库，就会不可逆销毁 mcp_server_oauth
// 里全组织共享的 refresh_token，所有人都得重跑浏览器授权。
func TestAuthorizeNeverDestroysCredential(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			authRequired := 0
			credentialInvalid := 0
			h := &oauthHandler{cfg: &OAuthConfig{
				OnAuthRequired:      func(error) { authRequired++ },
				OnCredentialInvalid: func(error) { credentialInvalid++ },
			}}

			err := h.Authorize(context.Background(), &http.Request{}, &http.Response{StatusCode: status})
			if err == nil {
				t.Fatal("Authorize should still report the failure so the turn's tool call fails")
			}
			if authRequired != 1 {
				t.Fatalf("OnAuthRequired called %d times, want 1 (the authorize button must appear)", authRequired)
			}
			if credentialInvalid != 0 {
				t.Fatalf("OnCredentialInvalid called %d times, want 0 — a bare %d says nothing about the credential; destroying the org-wide refresh token here is unrecoverable", credentialInvalid, status)
			}
		})
	}
}

// 回调未设置时不能 panic（CLI / 未挂 watch 的调用方）。
func TestAuthorizeWithoutCallbacks(t *testing.T) {
	h := &oauthHandler{cfg: &OAuthConfig{}}
	if err := h.Authorize(context.Background(), &http.Request{}, &http.Response{StatusCode: 401}); err == nil {
		t.Fatal("Authorize should report an error even without callbacks")
	}
}
