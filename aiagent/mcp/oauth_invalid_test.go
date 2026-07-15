package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"golang.org/x/oauth2"
)

// isCredentialInvalid 决定要不要把一台 MCP Server 的授权判死（进而清掉 token、弹重
// 授权按钮）。两个方向都必须准：
//   - 漏判 → 凭据已被撤销却不报，工具静默消失且没有按钮（本次要修的 bug）；
//   - 误判 → 一次网络抖动就把还能用的凭据抹掉，用户被迫重新授权。
func TestIsCredentialInvalid(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		// 明确的凭据失效：refresh token 被撤销/过期是最典型的一条
		{"invalid_grant", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}, true},
		{"invalid_client", &oauth2.RetrieveError{ErrorCode: "invalid_client"}, true},
		{"unauthorized_client", &oauth2.RetrieveError{ErrorCode: "unauthorized_client"}, true},
		{"token endpoint 401", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusUnauthorized}}, true},
		{"token endpoint 403", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusForbidden}}, true},
		{"wrapped invalid_grant", fmt.Errorf("refresh failed: %w", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}), true},

		// 瞬时故障：绝不能据此判死凭据
		{"token endpoint 500", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusInternalServerError}}, false},
		{"token endpoint 503", &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusServiceUnavailable}}, false},
		{"temporarily_unavailable", &oauth2.RetrieveError{ErrorCode: "temporarily_unavailable"}, false},
		{"plain network error", errors.New("dial tcp: i/o timeout"), false},
		{"context canceled", context.Canceled, false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCredentialInvalid(tc.err); got != tc.want {
				t.Fatalf("isCredentialInvalid(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
