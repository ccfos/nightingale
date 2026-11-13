package aop

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/toolkits/pkg/errorx"
)

func TestRecovery_PanicReturnsInternalServerErrorAndLogsStack(t *testing.T) {
	var logs bytes.Buffer
	router := gin.New()
	router.Use(RecoveryWithWriter(&logs))
	router.GET("/panic", func(c *gin.Context) { panic("unexpected failure") })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	request.Header.Set("Authorization", "Bearer private-token")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, logs.String(), "unexpected failure")
	require.Contains(t, logs.String(), "middleware_behavior_test.go")
	require.NotContains(t, logs.String(), "private-token")
}

func TestRecovery_PageErrorReturnsApplicationErrorAndRequestID(t *testing.T) {
	router := gin.New()
	router.Use(RecoveryWithWriter(nil))
	router.GET("/error", func(c *gin.Context) {
		c.Set("trace_id", "request-7")
		panic(errorx.PageError{Code: http.StatusOK, Message: "invalid input"})
	})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/error", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"err":"invalid input","request_id":"request-7"}`, recorder.Body.String())
}

func TestRecovery_PageErrorUsesRequestedHTTPStatus(t *testing.T) {
	router := gin.New()
	router.Use(RecoveryWithWriter(nil))
	router.GET("/denied", func(c *gin.Context) {
		panic(errorx.PageError{Code: http.StatusForbidden, Message: "access denied"})
	})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/denied", nil))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Equal(t, "access denied", recorder.Body.String())
}

func TestLogger_PreservesBodyAndReportsResponseMetadata(t *testing.T) {
	var formatted []LogFormatterParams
	config := LoggerConfig{
		Output: io.Discard, PrintAccessLog: func() bool { return true },
		PrintBodyPaths: func() map[string]struct{} { return map[string]struct{}{"/echo": {}} },
		Formatter: func(params LogFormatterParams) string {
			formatted = append(formatted, params)
			return "access record"
		},
	}
	router := gin.New()
	router.Use(LoggerWithConfig(config))
	router.POST("/echo", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		c.Set("custom", "value")
		_ = c.Error(errors.New("handler detail"))
		c.String(http.StatusCreated, "received: ")
		_, _ = c.Writer.Write(body)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/echo?format=text", strings.NewReader("payload"))

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusCreated, recorder.Code)
	require.Equal(t, "received: payload", recorder.Body.String())
	require.Len(t, formatted, 1)
	require.Equal(t, "/echo?format=text", formatted[0].Path)
	require.Equal(t, http.MethodPost, formatted[0].Method)
	require.Equal(t, http.StatusCreated, formatted[0].StatusCode)
	require.Equal(t, len("received: payload"), formatted[0].BodySize)
	require.Equal(t, "value", formatted[0].Keys["custom"])
	require.Contains(t, formatted[0].ErrorMessage, "handler detail")
}

func TestLogger_SkipsConfiguredPathsAndDisabledAccessLogs(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		skip    []string
	}{
		{name: "disabled", enabled: false},
		{name: "health endpoint", enabled: true, skip: []string{"/health"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := false
			router := gin.New()
			router.Use(LoggerWithConfig(LoggerConfig{
				Output: io.Discard, SkipPaths: tt.skip,
				PrintAccessLog: func() bool { return tt.enabled },
				PrintBodyPaths: func() map[string]struct{} { return nil },
				Formatter:      func(LogFormatterParams) string { formatted = true; return "unexpected log" },
			}))
			router.GET("/health", func(c *gin.Context) { c.String(http.StatusOK, "healthy") })
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

			require.Equal(t, "healthy", recorder.Body.String())
			require.False(t, formatted)
		})
	}
}

func TestReadBody_DecodesGzipPayload(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte("request body"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	body := readBody(&compressed, "gzip")

	require.Equal(t, "request body", body)
}

func TestReadBody_ReportsInvalidGzip(t *testing.T) {
	body := readBody(strings.NewReader("invalid gzip"), "gzip")

	require.Contains(t, body, "failed to create gzip reader")
}

func TestDefaultLogFormatter_RendersRequestAndError(t *testing.T) {
	previous := consoleColorMode
	t.Cleanup(func() { consoleColorMode = previous })
	DisableConsoleColor()
	params := LogFormatterParams{
		StatusCode: http.StatusNotFound, Method: http.MethodGet, Path: "/missing?q=1", ClientIP: "127.0.0.1",
		ErrorMessage: "missing resource",
	}

	line := defaultLogFormatter(params)

	require.Contains(t, line, "404")
	require.Contains(t, line, "GET")
	require.Contains(t, line, "/missing?q=1")
	require.Contains(t, line, "127.0.0.1")
	require.Contains(t, line, "missing resource")
	require.NotContains(t, line, "\x1b[")
}
