package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"golang.org/x/oauth2"
)

// classifyCredentialInvalid 决定要不要把一台 MCP Server 的凭据判死（进而清库、弹重
// 授权按钮），以及要清到什么程度。三个方向都必须准：
//   - 漏判 → 凭据已死却不报，工具静默消失（本功能要救的场景）；
//   - 误判 → 一次网关抖动/WAF 的裸 401 就把组织级 refresh token 抹掉，所有人重授权；
//   - 分类错 → invalid_client 却只清 token、留下已被拒的 client，重授权必然再次
//     invalid_client，陷入用户无法自愈的死循环。
func TestClassifyCredentialInvalid(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantKind CredentialInvalidKind
		wantDead bool
	}{
		// 授权被撤销：只有 grant 死了，client 注册仍可复用
		{"invalid_grant", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}, CredentialInvalidGrant, true},
		{"wrapped invalid_grant", fmt.Errorf("refresh failed: %w", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}), CredentialInvalidGrant, true},

		// client 本身被拒：必须连 client 注册一起作废，否则重授权是死循环
		{"invalid_client", &oauth2.RetrieveError{ErrorCode: "invalid_client"}, CredentialInvalidClient, true},
		{"unauthorized_client", &oauth2.RetrieveError{ErrorCode: "unauthorized_client"}, CredentialInvalidClient, true},

		// 裸 401/403（无 OAuth error body）：token endpoint 前的 WAF / IP 白名单 / 临时
		// 网关都可能这么回，据此判死会不可逆销毁一个完全正常的组织级 refresh token。
		{"bare 401 must not be fatal", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusUnauthorized}}, 0, false},
		{"bare 403 must not be fatal", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusForbidden}}, 0, false},

		// 其余瞬时故障同样不得判死
		{"token endpoint 500", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusInternalServerError}}, 0, false},
		{"token endpoint 503", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusServiceUnavailable}}, 0, false},
		{"temporarily_unavailable", &oauth2.RetrieveError{ErrorCode: "temporarily_unavailable"}, 0, false},
		{"invalid_scope is not credential death", &oauth2.RetrieveError{ErrorCode: "invalid_scope"}, 0, false},
		{"plain network error", errors.New("dial tcp: i/o timeout"), 0, false},
		{"context canceled", context.Canceled, 0, false},
		{"nil", nil, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, dead := classifyCredentialInvalid(tc.err)
			if dead != tc.wantDead {
				t.Fatalf("classifyCredentialInvalid(%v) dead = %v, want %v", tc.err, dead, tc.wantDead)
			}
			if dead && kind != tc.wantKind {
				t.Fatalf("classifyCredentialInvalid(%v) kind = %v, want %v", tc.err, kind, tc.wantKind)
			}
		})
	}
}

// Token() 是唯一允许判死凭据的路径，且必须带上分类。
func TestTokenSourceReportsCredentialInvalid(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantFire bool
		wantKind CredentialInvalidKind
	}{
		{"invalid_grant fires with grant kind", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}, true, CredentialInvalidGrant},
		{"invalid_client fires with client kind", &oauth2.RetrieveError{ErrorCode: "invalid_client"}, true, CredentialInvalidClient},
		{"bare 401 does not fire", &oauth2.RetrieveError{Response: &http.Response{StatusCode: 401}}, false, 0},
		{"network error does not fire", errors.New("i/o timeout"), false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fired := 0
			var gotKind CredentialInvalidKind
			p := &persistingTokenSource{
				src: failingTokenSource{err: tc.err},
				onCredentialInvalid: func(k CredentialInvalidKind, _ error) {
					fired++
					gotKind = k
				},
			}
			if _, err := p.Token(); err == nil {
				t.Fatal("Token() should propagate the failure")
			}
			if (fired > 0) != tc.wantFire {
				t.Fatalf("onCredentialInvalid fired %d times, want fired=%v", fired, tc.wantFire)
			}
			if tc.wantFire && gotKind != tc.wantKind {
				t.Fatalf("kind = %v, want %v", gotKind, tc.wantKind)
			}
		})
	}
}

type failingTokenSource struct{ err error }

func (f failingTokenSource) Token() (*oauth2.Token, error) { return nil, f.err }
