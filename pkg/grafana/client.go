// Package grafana 负责连接外部 Grafana 拉取其数据源清单，并把它们映射为
// n9e 的 models.Datasource，供「一键导入 Grafana 数据源」功能使用。
package grafana

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// 鉴权方式。Grafana 支持 Service Account Token（Bearer）与管理员用户名/密码（Basic）。
const (
	AuthTypeToken = "token"
	AuthTypeBasic = "basic"
)

// allowlistEnv 是运维放行内网/本地 Grafana 目标的白名单环境变量，逗号分隔的 IP 或 CIDR，
// 例如 "127.0.0.1/32,10.0.0.0/8"。默认只放行全局可路由的公网单播地址（见 targetBlocked），
// 内网/特殊网段一律拒绝，只有命中白名单才放行——secure-by-default，防止 SSRF 跳板。
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

// specialPrefixes 是 netip 分类方法未覆盖、但同样不应作为导入目标的特殊用途网段。
var specialPrefixes = parsePrefixes(
	"0.0.0.0/8",       // "this host on this network"（RFC 1122，0.0.0.0/32 之外的其余）
	"100.64.0.0/10",   // CGNAT (RFC 6598)
	"192.0.0.0/24",    // IETF 协议分配
	"192.0.2.0/24",    // TEST-NET-1 文档
	"198.18.0.0/15",   // 基准测试
	"198.51.100.0/24", // TEST-NET-2 文档
	"203.0.113.0/24",  // TEST-NET-3 文档
	"240.0.0.0/4",     // 保留/未来用途（含 255.255.255.255 广播）
	"64:ff9b:1::/48",  // IPv6 NAT64 本地用途
	"fec0::/10",       // IPv6 site-local（已废弃但仍可能在内网路由）
	"2001:db8::/32",   // IPv6 文档
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

// buildTargetURL 校验并规整用户填入的 Grafana 地址：只允许 http/https，丢弃 query 与
// fragment（防止在基址塞 query 绕开固定路径），保留可能的子路径后拼上 /api/datasources。
func buildTargetURL(raw string) (string, error) {
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
	return base.String() + "/api/datasources", nil
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

// targetBlocked 判断解析后的目标地址是否应被拦截。无效地址一律 fail closed；命中白名单放行；
// 否则只放行全局可路由的公网单播地址，其余（loopback / 私网+ULA / link-local / 组播 /
// unspecified / CGNAT 等特殊网段）全部拒绝。用 net/netip 分类，天然处理 IPv6 %zone。
func targetBlocked(addr netip.Addr, allow []netip.Prefix) bool {
	if !addr.IsValid() {
		return true
	}
	// 去掉 %zone 再归一 v4-mapped：既防 ::ffff:10.0.0.1 绕过，又让带 zone 的合法内网地址
	// （如 fe80::1%eth0）能被白名单 fe80::1/128 命中（netip.Prefix.Contains 不匹配带 zone 的地址）。
	addr = addr.WithZone("").Unmap()
	for _, p := range allow {
		if p.Contains(addr) {
			return false
		}
	}
	return !isGloballyRoutable(addr)
}

func isGloballyRoutable(addr netip.Addr) bool {
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	for _, p := range specialPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
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

// FetchDatasources 连接 Grafana 拉取其数据源清单。
func FetchDatasources(conn Conn) ([]GrafanaDatasource, error) {
	target, err := buildTargetURL(conn.URL)
	if err != nil {
		return nil, err
	}

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
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Timeout:       15 * time.Second,
		Transport:     transport,
		CheckRedirect: redirectPolicy,
	}

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
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, fmt.Errorf("grafana responded %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

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
