// Package grafana 负责连接外部 Grafana 拉取其数据源清单，并把它们映射为
// n9e 的 models.Datasource，供「一键导入 Grafana 数据源」功能使用。
package grafana

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

// 鉴权方式。Grafana 支持 Service Account Token（Bearer）与管理员用户名/密码（Basic）。
const (
	AuthTypeToken = "token"
	AuthTypeBasic = "basic"
)

// allowlistEnv 是可选的目标白名单环境变量，逗号分隔的 IP 或 CIDR，命中即强制放行（包括默认被拦的
// link-local / 元数据地址）。默认已放行 loopback / 私网 / 公网（Grafana 常与 n9e 同机或同内网），
// 仅拦截云元数据 / link-local / 组播等 SSRF 高危目标，故一般无需配置本变量。
const allowlistEnv = "N9E_GRAFANA_IMPORT_ALLOWLIST"

// maxErrorBodyBytes 是拼进错误信息里的响应正文上限。
const maxErrorBodyBytes = 2 << 10 // 2 KiB

var (
	// maxResponseBytes 限制读取 Grafana 响应的字节数，防止超大响应/压缩炸弹撑爆内存。var 便于测试下调。
	maxResponseBytes int64 = 10 << 20 // 10 MiB
	// maxDatasources 是单次拉取允许的数据源条数上限，流式解码时逐条计数、超限即止，
	// 防止「上限字节内塞几百万个最小元素」把切片撑到数百 MiB。
	maxDatasources = 10000
)

// metadataPrefixes 是即便默认放行内网、也仍要拦截的云厂商实例元数据地址（SSRF 最高危目标）。
// IPv4 169.254.169.254 属 link-local，已被 IsLinkLocalUnicast 拦；这里补 IPv6 元数据（属 ULA，
// 默认放行内网会漏）。
var metadataPrefixes = parsePrefixes(
	"fd00:ec2::254/128", // AWS IPv6 实例元数据
)

// Conn 描述一次连接 Grafana 的入参。
type Conn struct {
	URL           string `json:"url"`
	AuthType      string `json:"auth_type"`
	Token         string `json:"token"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
}

// GrafanaDatasource 是 Grafana GET /api/datasources 返回的单条数据源，
// 只保留导入需要的字段。注意 Grafana 出于安全不返回密钥明文，仅在 SecureJSONFields
// 里以布尔标记「哪些加密字段已配置」（且该字段通常只在单源详情返回、列表接口常缺省）。
type GrafanaDatasource struct {
	ID               int64                  `json:"id"`
	UID              string                 `json:"uid"`
	Name             string                 `json:"name"`
	Type             string                 `json:"type"`
	URL              string                 `json:"url"`
	Access           string                 `json:"access"`
	User             string                 `json:"user"`
	Database         string                 `json:"database"`
	BasicAuth        bool                   `json:"basicAuth"`
	BasicAuthUser    string                 `json:"basicAuthUser"`
	JSONData         map[string]interface{} `json:"jsonData"`
	SecureJSONFields map[string]bool        `json:"secureJsonFields"`
	IsDefault        bool                   `json:"isDefault"`
}

// buildBaseURL 校验并规整用户填入的 Grafana 地址：只允许 http/https，丢弃 query 与 fragment
// （防止在基址塞 query 绕开固定路径），保留可能的子路径，返回 scheme://host[/subpath]。
func buildBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("grafana url is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid grafana url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("grafana url must be http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("grafana url missing host")
	}
	base := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: strings.TrimRight(u.Path, "/")}
	return base.String(), nil
}

// buildTargetURL 返回数据源列表接口地址。
func buildTargetURL(raw string) (string, error) {
	base, err := buildBaseURL(raw)
	if err != nil {
		return "", err
	}
	return base + "/api/datasources", nil
}

func parsePrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		if p, err := netip.ParsePrefix(c); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// parseAllowlist 把白名单环境变量解析成 CIDR 列表（裸 IP 视作单机 /32 或 /128）。
func parseAllowlist(s string) []netip.Prefix {
	var out []netip.Prefix
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p.Masked())
			continue
		}
		if a, err := netip.ParseAddr(part); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out
}

// targetBlocked 判断解析后的目标地址是否应被拦截。无效地址一律 fail closed；命中白名单强制放行；
// 否则默认放行（含 loopback / 私网 / 公网——Grafana 常与 n9e 同机或同内网），只拦最危险的：
// link-local（含云元数据 169.254.169.254）、其它云元数据、组播、unspecified。
// 本功能仅 admin 可用，管理员本就能在「新增数据源」里指向任意地址，故对内网默认放行、只挡元数据跳板。
// 用 net/netip 分类，天然处理 IPv6 %zone。
func targetBlocked(addr netip.Addr, allow []netip.Prefix) bool {
	if !addr.IsValid() {
		return true
	}
	// 去掉 %zone 再归一 v4-mapped：既防 ::ffff:10.0.0.1 绕过，又让带 zone 的地址能被白名单命中。
	addr = addr.WithZone("").Unmap()
	for _, p := range allow {
		if p.Contains(addr) {
			return false
		}
	}
	if addr.IsUnspecified() || addr.IsMulticast() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return true
	}
	for _, p := range metadataPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// dialAddr 从 Dial Control 收到的 address（"ip:port"，IPv6 可能带 %zone）解析出目标地址。
func dialAddr(address string) (netip.Addr, bool) {
	if ap, err := netip.ParseAddrPort(address); err == nil {
		return ap.Addr(), true
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return a, true
	}
	return netip.Addr{}, false
}

// dialControl 返回 Dialer.Control：在 DNS 解析之后、实际连接之前校验真实目标 IP，
// 天然防 DNS rebinding；重定向后的每次拨号也会经过它。无法解析的目标一律拒绝（fail closed）。
func dialControl(allow []netip.Prefix) func(string, string, syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		addr, ok := dialAddr(address)
		if !ok {
			return fmt.Errorf("blocked unresolvable dial target %q", address)
		}
		if targetBlocked(addr, allow) {
			return fmt.Errorf("blocked non-public address %s; allowlist it via %s", addr, allowlistEnv)
		}
		return nil
	}
}

// countingReader 记录已读字节数，用于区分「正常 EOF」与「超限截断」。
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// redirectPolicy 约束重定向：最多 5 跳;禁止跨主机(防绕过 dialControl 与固定 /api/datasources 路径);
// 禁止 HTTPS→HTTP 降级——net/http 会向同主机重定向复制 Authorization 头,降级后 Bearer/Basic
// 凭据会以明文外发。
func redirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("stopped after too many redirects")
	}
	if len(via) == 0 {
		return nil
	}
	if req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("cross-host redirect blocked: %s", req.URL.Host)
	}
	if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing https->http downgrade redirect to host %s", req.URL.Host)
	}
	return nil
}

// describeConnError 把底层网络错误(dial/timeout/TLS/DNS)归类成面向用户的可读提示，
// 避免把 *url.Error/*net.OpError 结构体透传给前端(会被序列化成含 Op/Addr/Port 的原始 JSON)。
func describeConnError(err error) error {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("连接 Grafana 超时，请检查地址是否正确、网络是否可达")
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "x509"), strings.Contains(msg, "certificate"), strings.Contains(msg, "tls:"):
		return fmt.Errorf("Grafana TLS 证书校验失败，请使用受信任证书，或勾选『跳过 TLS 校验』")
	case strings.Contains(msg, "connection refused"):
		return fmt.Errorf("无法连接 Grafana(连接被拒绝)，请检查地址和端口")
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "server misbehaving"):
		return fmt.Errorf("无法解析 Grafana 域名，请检查地址是否正确")
	case strings.Contains(msg, "no route to host"), strings.Contains(msg, "network is unreachable"):
		return fmt.Errorf("无法连接 Grafana(网络不可达)，请检查地址与网络")
	case strings.Contains(msg, "blocked"):
		return fmt.Errorf("目标地址被安全策略拒绝(内网/环回/元数据地址)，如确需访问请配置白名单")
	default:
		return fmt.Errorf("连接 Grafana 失败，请检查地址与鉴权信息")
	}
}

// describeStatusError 把非 200 响应归类成可读提示，只保留一小段截断后的错误正文。
func describeStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	// 附上 Grafana 的原始错误正文(截断),让用户看到具体原因(如 Token 权限/org 不符)。
	suffix := ""
	if d := strings.TrimSpace(string(body)); d != "" {
		suffix = "；Grafana: " + d
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("Grafana 认证失败(401)，请检查 Token 或用户名/密码%s", suffix)
	case http.StatusForbidden:
		return fmt.Errorf("Grafana 拒绝访问(403)，Token/账号缺少 datasources:read 权限(建议用 Admin 角色的 Service Account)%s", suffix)
	case http.StatusNotFound:
		return fmt.Errorf("Grafana 接口未找到(404)，请确认填写的是 Grafana 根地址%s", suffix)
	default:
		return fmt.Errorf("Grafana 返回错误(%d)%s", resp.StatusCode, suffix)
	}
}

// FetchDatasources 连接 Grafana 拉取数据源清单。列表接口在部分版本里会省略 basicAuthUser /
// 完整 jsonData / secureJsonFields，故对「映射表命中(有望导入)」且带 uid 的数据源，再逐条拉
// /api/datasources/uid/:uid 详情回填这些字段——从而保留 Basic Auth 用户名，并用真实
// secureJsonFields 精确判定是否缺密钥。详情拉取失败不阻断，退回列表数据兜底。
func FetchDatasources(conn Conn) ([]GrafanaDatasource, error) {
	base, err := buildBaseURL(conn.URL)
	if err != nil {
		return nil, err
	}

	client, transport := newGrafanaClient(conn)
	defer transport.CloseIdleConnections()

	list, err := fetchDatasourceList(client, conn, base)
	if err != nil {
		return nil, err
	}
	enrichDatasourceDetails(client, conn, base, list)
	return list, nil
}

// newGrafanaClient 构建带 SSRF 校验(dialControl)、禁降级重定向、可选跳过 TLS 的 http.Client，
// 返回 transport 供调用方 CloseIdleConnections。
func newGrafanaClient(conn Conn) (*http.Client, *http.Transport) {
	allow := parseAllowlist(os.Getenv(allowlistEnv))
	transport := &http.Transport{
		// 刻意不启用 http.ProxyFromEnvironment：走代理时 Control 校验到的是代理地址而非
		// 最终目标，SSRF 校验会被绕过。宁可不支持代理，也要让 IP 校验作用于真实目标。
		IdleConnTimeout: 30 * time.Second,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
			Control: dialControl(allow),
		}).DialContext,
	}
	if conn.SkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{
		Timeout:       15 * time.Second,
		Transport:     transport,
		CheckRedirect: redirectPolicy,
	}, transport
}

// grafanaGet 发起一次带鉴权的 GET，网络错误与非 200 都归类成可读错误。成功时返回 resp，
// 由调用方负责关闭 Body。
func grafanaGet(client *http.Client, conn Conn, target string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if conn.AuthType == AuthTypeBasic {
		req.SetBasicAuth(conn.Username, conn.Password)
	} else {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(conn.Token))
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, describeConnError(err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, describeStatusError(resp)
	}
	return resp, nil
}

// fetchDatasourceList 拉取并流式解码数据源列表，带字节与条数上限。
func fetchDatasourceList(client *http.Client, conn Conn, base string) ([]GrafanaDatasource, error) {
	resp, err := grafanaGet(client, conn, base+"/api/datasources")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 多读 1 字节以区分「恰好到上限的合法响应」与「超限截断」。计数器超过上限即判 too large。
	cr := &countingReader{r: io.LimitReader(resp.Body, maxResponseBytes+1)}
	lst, err := decodeDatasources(cr)
	if cr.n > maxResponseBytes {
		return nil, fmt.Errorf("grafana response exceeds %d bytes", maxResponseBytes)
	}
	if err != nil {
		return nil, err
	}
	return lst, nil
}

// fetchDatasourceDetail 拉取单条数据源详情(含 basicAuthUser / 完整 jsonData / secureJsonFields)。
func fetchDatasourceDetail(client *http.Client, conn Conn, base, uid string) (GrafanaDatasource, error) {
	var gds GrafanaDatasource
	resp, err := grafanaGet(client, conn, base+"/api/datasources/uid/"+url.PathEscape(uid))
	if err != nil {
		return gds, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return gds, err
	}
	if int64(len(body)) > maxResponseBytes {
		return gds, fmt.Errorf("grafana datasource detail exceeds %d bytes", maxResponseBytes)
	}
	if err := json.Unmarshal(body, &gds); err != nil {
		return gds, fmt.Errorf("decode grafana datasource detail: %w", err)
	}
	return gds, nil
}

// enrichDatasourceDetails 对映射表命中且带 uid 的数据源，并发拉详情回填非密字段。
// 单条失败静默跳过(保留列表数据兜底)，不阻断整体拉取；并发上限固定，避免数据源很多时压垮 Grafana。
func enrichDatasourceDetails(client *http.Client, conn Conn, base string, list []GrafanaDatasource) {
	const concurrency = 6
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range list {
		if list[i].UID == "" || !isMappedType(list[i].Type) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			// 各 goroutine 只写自己那个下标，元素间无共享内存，无需加锁。
			if detail, err := fetchDatasourceDetail(client, conn, base, list[i].UID); err == nil {
				mergeDetail(&list[i], detail)
			}
		}(i)
	}
	wg.Wait()
}

// mergeDetail 用详情里更完整的非密字段覆盖列表项；只回填有值的字段，避免用空值清掉列表已有数据。
func mergeDetail(dst *GrafanaDatasource, detail GrafanaDatasource) {
	dst.BasicAuth = detail.BasicAuth || dst.BasicAuth
	if detail.BasicAuthUser != "" {
		dst.BasicAuthUser = detail.BasicAuthUser
	}
	if detail.User != "" {
		dst.User = detail.User
	}
	if detail.Database != "" {
		dst.Database = detail.Database
	}
	if len(detail.JSONData) > 0 {
		dst.JSONData = detail.JSONData
	}
	if len(detail.SecureJSONFields) > 0 {
		dst.SecureJSONFields = detail.SecureJSONFields
	}
}

// decodeDatasources 逐条流式解码 JSON 数组，超过 maxDatasources 立即终止；
// 结束后必须读到数组结束符 ']' 且其后为 EOF，否则视为截断/尾随污染而报错，绝不静默返回部分列表。
func decodeDatasources(r io.Reader) ([]GrafanaDatasource, error) {
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("decode grafana datasources: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, fmt.Errorf("unexpected grafana response: not a JSON array")
	}

	lst := make([]GrafanaDatasource, 0, 64)
	for dec.More() {
		if len(lst) >= maxDatasources {
			return nil, fmt.Errorf("grafana returned more than %d datasources", maxDatasources)
		}
		var gds GrafanaDatasource
		if err := dec.Decode(&gds); err != nil {
			return nil, fmt.Errorf("decode grafana datasource: %w", err)
		}
		lst = append(lst, gds)
	}

	// 必须读到收尾的 ']'——若响应在完整元素后被截断（如 `[{},`），More() 遇 EOF 也返回 false，
	// 这里的 Token() 会报错，从而暴露截断而非把部分列表当成功。
	tok, err = dec.Token()
	if err != nil {
		return nil, fmt.Errorf("grafana response truncated: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != ']' {
		return nil, fmt.Errorf("unexpected grafana response: expected ']'")
	}
	// 数组之后必须是 EOF，杜绝尾随内容。
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing content after grafana datasources array")
	}
	return lst, nil
}
