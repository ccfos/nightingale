package ginx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func ginTestContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	c.Set("trace_id", "trace-test")
	return c, w
}

func TestBasicAuth_AcceptsCredentialsAndExposesUser(t *testing.T) {
	c, w := ginTestContext("/")
	c.Request.SetBasicAuth("alice", "secret")
	middleware := BasicAuth(Accounts{{User: "alice", Password: "secret"}})

	middleware(c)

	require.False(t, c.IsAborted())
	require.Equal(t, "alice", c.GetString(AuthUserKey))
	require.Empty(t, w.Header().Get("WWW-Authenticate"))
}

func TestBasicAuth_RejectsMissingOrWrongCredentials(t *testing.T) {
	cases := []struct {
		name, authorization, realm, want string
	}{
		{name: "missing", want: `Basic realm="Authorization Required"`},
		{name: "wrong", authorization: "Basic invalid", realm: "monitoring", want: `Basic realm="monitoring"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := ginTestContext("/")
			c.Request.Header.Set("Authorization", tc.authorization)
			middleware := BasicAuthForRealm(Accounts{{User: "alice", Password: "secret"}}, tc.realm)

			middleware(c)

			require.True(t, c.IsAborted())
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Equal(t, tc.want, w.Header().Get("WWW-Authenticate"))
			require.Empty(t, c.GetString(AuthUserKey))
		})
	}
}

func TestBasicAuth_RejectsInvalidAccountConfiguration(t *testing.T) {
	require.PanicsWithValue(t, "Empty list of authorized credentials", func() { BasicAuth(nil) })
	require.PanicsWithValue(t, "User can not be empty", func() { BasicAuth(Accounts{{Password: "secret"}}) })
}

func TestRenderMessage_FormatsResponseAndTrace(t *testing.T) {
	cases := []struct {
		name string
		code int
		msg  interface{}
		want string
	}{
		{name: "nil success", code: 200, want: `{"err":"","request_id":"trace-test"}`},
		{name: "string success", code: 200, msg: "invalid %s", want: `{"err":"invalid value","request_id":"trace-test"}`},
		{
			name: "error success", code: 200, msg: errors.New("invalid %s"),
			want: `{"err":"invalid value","request_id":"trace-test"}`,
		},
		{name: "nil failure", code: 400},
		{name: "string failure", code: 400, msg: "invalid %s", want: "invalid value"},
		{name: "error failure", code: 500, msg: errors.New("invalid %s"), want: "invalid value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := ginTestContext("/")

			NewRender(c, tc.code).Message(tc.msg, "value")

			require.Equal(t, tc.code, w.Code)
			if tc.code == 200 {
				require.JSONEq(t, tc.want, w.Body.String())
			} else {
				require.Equal(t, tc.want, w.Body.String())
			}
		})
	}
}

func TestRenderData_IncludesPayload(t *testing.T) {
	c, w := ginTestContext("/")

	NewRender(c).Data([]string{"api"}, nil)

	require.Equal(t, 200, w.Code)
	require.JSONEq(t, `{"dat":["api"],"err":"","request_id":"trace-test"}`, w.Body.String())
}

func TestRenderData_ReportsError(t *testing.T) {
	c, w := ginTestContext("/")

	NewRender(c, 422).Data("ignored", errors.New("invalid input"))

	require.Equal(t, 422, w.Code)
	require.Equal(t, "invalid input", w.Body.String())
}

func TestRenderZeroPage_UsesEmptyArray(t *testing.T) {
	c, w := ginTestContext("/")

	NewRender(c).ZeroPage()

	require.JSONEq(t, `{"dat":{"list":[],"total":0},"err":"","request_id":"trace-test"}`, w.Body.String())
}

func TestBindJSON_DecodesAndValidatesBody(t *testing.T) {
	cases := []struct {
		name, body string
		wantPanic  bool
	}{
		{name: "valid", body: `{"name":"alice"}`},
		{name: "malformed", body: `{`, wantPanic: true},
		{name: "missing required", body: `{}`, wantPanic: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := ginTestContext("/")
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var body struct {
				Name string `json:"name" binding:"required"`
			}

			if tc.wantPanic {
				require.Panics(t, func() { BindJSON(c, &body) })
			} else {
				BindJSON(c, &body)

				require.Equal(t, "alice", body.Name)
			}
		})
	}
}

func TestURLParams_ParseValuesAndRejectInvalidNumbers(t *testing.T) {
	c, _ := ginTestContext("/")
	c.Params = gin.Params{{Key: "id", Value: "42"}, {Key: "bad", Value: "abc"}}

	require.Equal(t, "42", UrlParamStr(c, "id"))
	require.Equal(t, int64(42), UrlParamInt64(c, "id"))
	require.Equal(t, 42, UrlParamInt(c, "id"))
	require.Panics(t, func() { UrlParamStr(c, "missing") })
	require.Panics(t, func() { UrlParamInt64(c, "bad") })
}

func TestQueryParams_ParseDefaultsAndRejectInvalidValues(t *testing.T) {
	c, _ := ginTestContext("/?name=alice&count=42&large=9223372036854775807&bad=abc")

	require.Equal(t, "alice", QueryStr(c, "name"))
	require.Equal(t, "fallback", QueryStr(c, "missing", "fallback"))
	require.Equal(t, 42, QueryInt(c, "count"))
	require.Equal(t, 10, QueryInt(c, "missing", 10))
	require.Equal(t, int64(9223372036854775807), QueryInt64(c, "large"))
	require.Equal(t, int64(10), QueryInt64(c, "missing", 10))
	require.Panics(t, func() { QueryStr(c, "missing") })
	require.Panics(t, func() { QueryInt(c, "missing") })
	require.Panics(t, func() { QueryInt(c, "bad") })
	require.Panics(t, func() { QueryInt64(c, "missing") })
	require.Panics(t, func() { QueryInt64(c, "bad") })
}

func TestQueryBool_RecognizesSupportedSpellings(t *testing.T) {
	for _, value := range []string{"true", "1", "on", "checked", "yes", "Y", "false", "0", "off", "no", "N"} {
		t.Run(value, func(t *testing.T) {
			c, _ := ginTestContext("/?enabled=" + value)

			got := QueryBool(c, "enabled")

			want := value == "true" || value == "1" || value == "on" || value == "checked" || value == "yes" || value == "Y"
			require.Equal(t, want, got)
		})
	}
}

func TestQueryBool_DefaultsAndRejectsUnknownValues(t *testing.T) {
	c, _ := ginTestContext("/?bad=maybe")

	require.True(t, QueryBool(c, "missing", true))
	require.Panics(t, func() { QueryBool(c, "missing") })
	require.Panics(t, func() { QueryBool(c, "bad") })
}

func TestOffset_UsesPageAndDefaultLimit(t *testing.T) {
	c, _ := ginTestContext("/?p=3&page=4")

	require.Equal(t, 40, Offset(c, 20))
	require.Equal(t, 20, Offset(c, 0))
	require.Equal(t, 60, Offset(c, 20, "page"))
}
