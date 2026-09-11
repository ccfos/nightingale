package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/gin-gonic/gin"
)

// TestTargetsOfAlertRuleFromCache 覆盖 /v1/n9e/targets-of-alert-rule 走内存缓存的路径：
// 只回请求的引擎、其他引擎的数据不外泄；引擎没有绑定任何机器时返回空对象而不是报错。
// 该路径不碰 DB，所以 Router 只需挂上缓存即可。
func TestTargetsOfAlertRuleFromCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cache := &memsto.TargetsOfAlertRuleCacheType{}
	cache.Set(map[string]map[int64][]string{
		"edge-a": {1: {"host-1", "host-2"}, 2: {"host-3"}},
		"edge-b": {1: {"host-9"}},
	}, 0, 0)
	rt := &Router{TargetsOfAlertRuleCache: cache}

	cases := []struct {
		name   string
		engine string
		want   map[string]map[int64][]string
	}{
		{"只返回请求的引擎", "edge-a", map[string]map[int64][]string{"edge-a": {1: {"host-1", "host-2"}, 2: {"host-3"}}}},
		{"未知引擎返回空对象", "edge-none", map[string]map[int64][]string{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/n9e/targets-of-alert-rule?engine_name="+tc.engine, nil)

			rt.targetsOfAlertRule(c)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
			}
			var resp struct {
				Dat map[string]map[int64][]string `json:"dat"`
				Err string                        `json:"err"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal body %s: %v", w.Body.String(), err)
			}
			if resp.Err != "" {
				t.Fatalf("err = %q, want empty", resp.Err)
			}
			if !reflect.DeepEqual(resp.Dat, tc.want) {
				t.Fatalf("dat = %v, want %v", resp.Dat, tc.want)
			}
		})
	}
}
