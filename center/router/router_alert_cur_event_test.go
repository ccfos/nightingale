package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestReadEventIds 覆盖事件 id 过滤参数的读取：GET 走 query、POST 走 body（body 优先），
// 二者都缺时返回空切片。POST 走 body 是为了避免数千个事件 id 拼到 URL 导致 nginx 414。
func TestReadEventIds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name        string
		method      string
		queryString string
		body        string
		want        []int64
	}{
		{"GET 从 query 读取", http.MethodGet, "event_ids=1,2,3", "", []int64{1, 2, 3}},
		{"POST 从 body 读取", http.MethodPost, "", `{"event_ids":"4,5,6"}`, []int64{4, 5, 6}},
		{"POST body 为空时回退 query", http.MethodPost, "event_ids=7,8", `{}`, []int64{7, 8}},
		{"POST body 优先于 query", http.MethodPost, "event_ids=1", `{"event_ids":"9,10"}`, []int64{9, 10}},
		{"无参数返回空切片", http.MethodGet, "", "", []int64{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tc.method, "/?"+tc.queryString, bytes.NewBufferString(tc.body))
			if tc.body != "" {
				c.Request.Header.Set("Content-Type", "application/json")
			}

			if got := readEventIds(c); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("readEventIds() = %v, want %v", got, tc.want)
			}
		})
	}
}
