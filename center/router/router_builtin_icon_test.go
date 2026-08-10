package router

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/center/cconf"

	"github.com/gin-gonic/gin"
)

// 老版本 DB 的 builtin_components.logo 存过与磁盘目录大小写/拼写不一致的路径
// （ElasticSearch vs Elasticsearch、Tecent vs Tencent），Init 更新存量组件不刷
// Logo，这些请求只能靠 builtinIcon 的兜底逻辑救活。
func TestBuiltinIcon(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "Elasticsearch", "icon", "elasticsearch.png"), "png-es")
	mustWriteFile(t, filepath.Join(dir, "Tencent", "icon", "tencent-cloud.svg"), "svg-tencent")

	rt := &Router{Center: cconf.Center{BuiltinIntegrationsDir: dir}}

	cases := []struct {
		name     string
		cate     string
		file     string
		wantCode int
		wantBody string
	}{
		{"exact path", "Elasticsearch", "elasticsearch.png", 200, "png-es"},
		{"case-insensitive cate fallback", "ElasticSearch", "elasticsearch.png", 200, "png-es"},
		{"filename fallback to first icon", "ElasticSearch", "renamed.png", 200, "png-es"},
		{"spelling alias Tecent", "Tecent", "tecent.png", 200, "svg-tencent"},
		{"unknown cate is 404", "NoSuchComponent", "x.png", 404, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/n9e/integrations/icon/"+tc.cate+"/"+tc.file, nil)
			c.Params = gin.Params{{Key: "cate", Value: tc.cate}, {Key: "name", Value: tc.file}}

			rt.builtinIcon(c)

			if w.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d, body: %s", w.Code, tc.wantCode, w.Body.String())
			}
			if tc.wantBody != "" && w.Body.String() != tc.wantBody {
				t.Fatalf("body = %q, want %q", w.Body.String(), tc.wantBody)
			}
		})
	}

	for _, tc := range []struct{ cate, file string }{
		{"..", "passwd"},
		{"Elasticsearch", "../../../../etc/passwd"},
	} {
		t.Run("traversal rejected "+tc.cate+"/"+tc.file, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/n9e/integrations/icon/x/y", nil)
			c.Params = gin.Params{{Key: "cate", Value: tc.cate}, {Key: "name", Value: tc.file}}

			defer func() {
				if recover() == nil {
					t.Fatalf("expected Bomb panic for %q/%q", tc.cate, tc.file)
				}
			}()
			rt.builtinIcon(c)
		})
	}
}

func mustWriteFile(t *testing.T, fp, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
