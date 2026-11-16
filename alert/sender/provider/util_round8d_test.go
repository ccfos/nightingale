package provider

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestR8dRetryCancellationPreservesLastFailure(t *testing.T) {
	c, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, got := newCaptureServer(t, always(503, `busy`))
	response, err := doWithRetry(c, srv.Client(), 1, time.Hour, func(c context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(c, "POST", srv.URL, nil)
	}, func(r *nativeResponse) (bool, error) {
		r.Header.Set("Retry-After", "3600")
		cancel()
		return true, errors.New("upstream busy")
	})
	require.ErrorContains(t, err, "upstream busy")
	require.ErrorContains(t, err, "canceled after 1 attempts")
	require.Equal(t, 503, response.StatusCode)
	require.Len(t, got(), 1)
	_, err = doWithRetry(nil, srv.Client(), 0, 0, func(context.Context) (*http.Request, error) { return nil, errors.New("build rejected") }, nil)
	require.EqualError(t, err, "build rejected")
	_, err = doWithRetry(nil, nil, 0, 0, nil, nil)
	require.EqualError(t, err, "http client not found")
}
func TestR8dRetryAfterAndUnicodeBoundaries(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		input string
		want  time.Duration
		ok    bool
	}{{"", 0, false}, {"-1", 0, false}, {"bad", 0, false}, {now.Add(-time.Hour).Format(http.TimeFormat), 0, true}, {"1.5", 1500 * time.Millisecond, true}} {
		d, ok := parseRetryAfter(tc.input, now)
		require.Equal(t, tc.ok, ok)
		require.Equal(t, tc.want, d)
	}
	for _, n := range []int{-1, 0, 1, 2, 3} {
		s, cut := truncateInRunes("你好世界", n)
		require.True(t, cut)
		if n <= 0 {
			require.Empty(t, s)
		} else {
			require.Equal(t, []rune("你好世界")[:n], []rune(s))
		}
	}
	s, cut := truncateInUTF16("abc", 10)
	require.False(t, cut)
	require.Equal(t, "abc", s)
	cause := errors.New("cause")
	require.Equal(t, "cause", (&HintError{Err: cause}).Error())
	require.Empty(t, LocalizeError(nil, nil))
	require.Equal(t, "cause", LocalizeError(cause, nil))
	require.Empty(t, getParsedString("bad", "{{", nil))
	require.Empty(t, getParsedHTTPString("bad", "{{", nil))
	require.Contains(t, getParsedString("bad", "{{call .x}}", map[string]interface{}{"x": 1}), "failed to parse template")
	require.Contains(t, getParsedHTTPString("bad", "{{call .x}}", map[string]interface{}{"x": 1}), "failed to parse template")
}
