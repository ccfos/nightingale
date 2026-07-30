package poster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

func TestPostByUrls(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DataResponse[interface{}]{Dat: "", Err: ""}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	ctx := &ctx.Context{
		CenterApi: conf.CenterApi{
			Addrs: []string{server.URL},
		}}

	if err := PostByUrls(ctx, "/v1/n9e/server-heartbeat", map[string]string{"a": "aa"}); err != nil {
		t.Errorf("PostByUrls() error = %v ", err)
	}
}

func TestPostByUrlsWithResp(t *testing.T) {

	expected := int64(123)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DataResponse[int64]{Dat: expected, Err: ""}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	ctx := &ctx.Context{
		CenterApi: conf.CenterApi{
			Addrs: []string{server.URL},
		}}

	gotT, err := PostByUrlsWithResp[int64](ctx, "/v1/n9e/event-persist", map[string]string{"b": "bb"})
	if err != nil {
		t.Errorf("PostByUrlsWithResp() error = %v", err)
		return
	}
	if gotT != expected {
		t.Errorf("PostByUrlsWithResp() gotT = %v,expected = %v", gotT, expected)
	}

}

func TestPostByUrlsWithRespRetry(t *testing.T) {
	expected := int64(123)

	t.Run("succeeds after transient failures", func(t *testing.T) {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 前两轮返回 500，第三轮才成功
			if atomic.AddInt32(&calls, 1) < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(DataResponse[int64]{Dat: expected})
		}))
		defer server.Close()

		c := &ctx.Context{
			CenterApi: conf.CenterApi{
				Addrs: []string{server.URL},
			}}

		gotT, err := PostByUrlsWithRespRetry[int64](c, "/v1/n9e/event-persist",
			map[string]string{"b": "bb"}, 3, time.Millisecond)
		if err != nil {
			t.Fatalf("PostByUrlsWithRespRetry() error = %v", err)
		}
		if gotT != expected {
			t.Errorf("PostByUrlsWithRespRetry() gotT = %v, expected = %v", gotT, expected)
		}
		if got := atomic.LoadInt32(&calls); got != 3 {
			t.Errorf("PostByUrlsWithRespRetry() calls = %d, expected = 3", got)
		}
	})

	t.Run("exhausts rounds", func(t *testing.T) {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		c := &ctx.Context{
			CenterApi: conf.CenterApi{
				// 两个地址：每轮都会把两个地址各试一次
				Addrs: []string{server.URL, server.URL},
			}}

		_, err := PostByUrlsWithRespRetry[int64](c, "/v1/n9e/event-persist",
			map[string]string{"b": "bb"}, 2, time.Millisecond)
		if err == nil {
			t.Fatal("PostByUrlsWithRespRetry() error = nil, expected failure")
		}
		if !strings.Contains(err.Error(), "after 2 rounds") {
			t.Errorf("PostByUrlsWithRespRetry() error = %v, expected it to mention the round count", err)
		}
		if got := atomic.LoadInt32(&calls); got != 4 {
			t.Errorf("PostByUrlsWithRespRetry() calls = %d, expected = 4 (2 rounds x 2 addrs)", got)
		}
	})

	t.Run("stops when context is canceled", func(t *testing.T) {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		stdCtx, cancel := context.WithCancel(context.Background())
		cancel()

		c := &ctx.Context{
			Ctx: stdCtx,
			CenterApi: conf.CenterApi{
				Addrs: []string{server.URL},
			}}

		_, err := PostByUrlsWithRespRetry[int64](c, "/v1/n9e/event-persist",
			map[string]string{"b": "bb"}, 5, time.Hour)
		if err == nil {
			t.Fatal("PostByUrlsWithRespRetry() error = nil, expected failure")
		}
		if !strings.Contains(err.Error(), "context done") {
			t.Errorf("PostByUrlsWithRespRetry() error = %v, expected it to report the canceled wait", err)
		}
		// 第一轮跑完就应该在等待处退出，不该进入第二轮
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("PostByUrlsWithRespRetry() calls = %d, expected = 1", got)
		}
	})

	t.Run("no addrs configured is not retried", func(t *testing.T) {
		c := &ctx.Context{}

		_, err := PostByUrlsWithRespRetry[int64](c, "/v1/n9e/event-persist",
			map[string]string{"b": "bb"}, 3, time.Hour)
		if err == nil {
			t.Fatal("PostByUrlsWithRespRetry() error = nil, expected failure")
		}
		if !strings.Contains(err.Error(), "ctx.CenterApi.Addrs") {
			t.Errorf("PostByUrlsWithRespRetry() error = %v, expected the PostByUrlsWithResp error", err)
		}
	})
}

func TestPostWithCenterApiTimeoutOverride(t *testing.T) {
	expected := int64(123)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 比收窄后的超时长、比配置的超时短：只有覆盖生效时这次请求才会超时
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(DataResponse[int64]{Dat: expected})
	}))
	defer server.Close()

	c := &ctx.Context{
		CenterApi: conf.CenterApi{
			Addrs:   []string{server.URL},
			Timeout: 9000,
		}}

	short := c.WithCenterApiTimeout(50)
	if short.CenterApi.Timeout != 50 {
		t.Fatalf("WithCenterApiTimeout() Timeout = %d, expected = 50", short.CenterApi.Timeout)
	}
	if c.CenterApi.Timeout != 9000 {
		t.Fatalf("WithCenterApiTimeout() mutated the original ctx, Timeout = %d", c.CenterApi.Timeout)
	}

	if _, err := PostByUrlsWithRespRetry[int64](short, "/v1/n9e/event-persist",
		map[string]string{"b": "bb"}, 1, time.Millisecond); err == nil {
		t.Error("PostByUrlsWithRespRetry() error = nil, expected the tightened timeout to fire")
	}

	gotT, err := PostByUrlsWithRespRetry[int64](c, "/v1/n9e/event-persist",
		map[string]string{"b": "bb"}, 1, time.Millisecond)
	if err != nil {
		t.Fatalf("PostByUrlsWithRespRetry() error = %v", err)
	}
	if gotT != expected {
		t.Errorf("PostByUrlsWithRespRetry() gotT = %v, expected = %v", gotT, expected)
	}
}
