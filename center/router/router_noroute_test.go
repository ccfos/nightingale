package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/runner"
)

// newFixturePub 在临时目录里造一个最小的前端发布包，返回它的父目录（即 pub 的上一级）
func newFixturePub(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	files := map[string]string{
		"pub/index.html": "<!doctype html>\n<html><body>n9e spa</body></html>\n",
		"pub/n9e-collect-templates/nvidia_smi.toml": "# nvidia_smi\n[[instances]]\nlabels = { job = \"nvidia_smi\" }\n",
		"pub/font/Inter-latin.woff2":                "wOF2-fake-font-bytes",
	}

	for name, content := range files {
		fpath := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", fpath, err)
		}
		if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", fpath, err)
		}
	}

	return root
}

type noRouteCase struct {
	name       string
	path       string
	wantStatus int
	wantBody   string // 响应体必须包含的片段，为空表示不校验
}

// 内嵌资源、磁盘 pub 目录两种模式共用同一份期望
func noRouteCases() []noRouteCase {
	return []noRouteCase{
		// issue #3317：内置采集模板是 toml，不在后缀列表里时会被当成前端路由返回 index.html
		{"builtin collect template", "/n9e-collect-templates/nvidia_smi.toml", http.StatusOK, "[[instances]]"},
		// 同一批漏掉的还有字体 woff2
		{"woff2 font", "/font/Inter-latin.woff2", http.StatusOK, "wOF2"},
		// 资源确实不存在时必须 404：前端 loadTemplate 判的是 res.ok，
		// 返回 200 + index.html 会把整个 HTML 当模板内容塞进 toml 编辑器
		{"missing collect template", "/n9e-collect-templates/not-exist.toml", http.StatusNotFound, ""},
		// 前端路由（含带点的 ident）依旧交给 SPA
		{"spa route", "/alert-rules/1", http.StatusOK, "n9e spa"},
		{"spa route with dots", "/targets/10.99.1.107", http.StatusOK, "n9e spa"},
	}
}

func runNoRouteCases(t *testing.T, r *gin.Engine) {
	t.Helper()

	for _, tc := range noRouteCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("path %s: status = %d, want %d, body=%q", tc.path, w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantBody != "" && !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Fatalf("path %s: body = %q, want contains %q", tc.path, w.Body.String(), tc.wantBody)
			}
		})
	}
}

// 内嵌资源模式：用 http.Dir 顶替 statik 生成的 FileSystem
func TestConfigNoRoute_EmbeddedAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rt := &Router{Center: cconf.Center{UseFileAssets: false}}
	fs := http.FileSystem(http.Dir(filepath.Join(newFixturePub(t), "pub")))

	r := gin.New()
	rt.configNoRoute(r, &fs)

	runNoRouteCases(t, r)
}

// 文件资源模式：把 runner.Cwd 指到临时目录，pub 目录即落在它下面
func TestConfigNoRoute_FileAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := newFixturePub(t)

	old := runner.Cwd
	runner.Cwd = root
	defer func() { runner.Cwd = old }()

	rt := &Router{Center: cconf.Center{UseFileAssets: true}}
	fs := http.FileSystem(http.Dir(filepath.Join(root, "pub")))

	r := gin.New()
	rt.configNoRoute(r, &fs)

	runNoRouteCases(t, r)
}
