package victorialogs

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	dskittypes "github.com/ccfos/nightingale/v6/dskit/types"
)

// maxLogFieldSize 单个日志字段值的字节上限，超出部分截断。
// 取 64KB 是因为它对一条 Java 堆栈足够（数百个栈帧），同时能挡住把上百 MB
// 的单行日志原样甩给浏览器或 LLM。
const maxLogFieldSize = 64 * 1024

// maxErrLineSnippet 解析失败时回传的原始行片段上限。定位一处 JSON 语法错误 1KB 足够，
// 再大就是把整段响应体写进 HTTP 响应和日志。
const maxErrLineSnippet = 1024

type VictoriaLogs struct {
	VictorialogsAddr  string `json:"victorialogs.addr" mapstructure:"victorialogs.addr"`
	VictorialogsBasic struct {
		VictorialogsUser string `json:"victorialogs.user" mapstructure:"victorialogs.user"`
		VictorialogsPass string `json:"victorialogs.password" mapstructure:"victorialogs.password"`
		IsEncrypt        bool   `json:"victorialogs.is_encrypt" mapstructure:"victorialogs.is_encrypt"`
	} `json:"victorialogs.basic" mapstructure:"victorialogs.basic"`
	VictorialogsTls struct {
		SkipTlsVerify bool `json:"victorialogs.tls.skip_tls_verify" mapstructure:"victorialogs.tls.skip_tls_verify"`
	} `json:"victorialogs.tls" mapstructure:"victorialogs.tls"`
	Headers      map[string]string `json:"victorialogs.headers" mapstructure:"victorialogs.headers"`
	Timeout      int64             `json:"victorialogs.timeout" mapstructure:"victorialogs.timeout"` // millis
	ClusterName  string            `json:"victorialogs.cluster_name" mapstructure:"victorialogs.cluster_name"`
	MaxQueryRows int               `json:"victorialogs.max_query_rows" mapstructure:"victorialogs.max_query_rows"`
	EnableWrite  bool              `json:"victorialogs.enable_write" mapstructure:"victorialogs.enable_write"`
	WriteAddrs   []string          `json:"victorialogs.write_addrs" mapstructure:"victorialogs.write_addrs"`

	HTTPClient *http.Client `json:"-" mapstructure:"-"`
}

// LogEntry 日志条目
type LogEntry map[string]interface{}

// PrometheusResponse Prometheus 响应格式
type PrometheusResponse struct {
	Status string         `json:"status"`
	Data   PrometheusData `json:"data"`
	Error  string         `json:"error,omitempty"`
}

// PrometheusData Prometheus 数据部分
type PrometheusData struct {
	ResultType string           `json:"resultType"`
	Result     []PrometheusItem `json:"result"`
}

// PrometheusItem Prometheus 数据项
type PrometheusItem struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value,omitempty"`  // [timestamp, value]
	Values [][]interface{}   `json:"values,omitempty"` // [[timestamp, value], ...]
}

// HitsResult hits 查询响应
type HitsResult struct {
	Hits []HitResult `json:"hits"`
}

type HitResult struct {
	Total      int64             `json:"total"`
	Timestamps []interface{}     `json:"timestamps"`
	Values     []interface{}     `json:"values"`
	Fields     map[string]string `json:"fields"`
}

type streamValuesResponse struct {
	Values []StreamFieldValue `json:"values"`
}

type StreamFieldValue struct {
	Value string `json:"value"`
	Hits  int64  `json:"hits"`
}

// InitHTTPClient 初始化 HTTP 客户端
func (vl *VictoriaLogs) InitHTTPClient() error {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: vl.VictorialogsTls.SkipTlsVerify,
		},
	}

	timeout := time.Duration(vl.Timeout) * time.Millisecond
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	vl.HTTPClient = &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return nil
}

// Query 执行日志查询
// GET/POST /select/logsql/query?query=<query>&start=<start>&end=<end>&limit=<limit>
func (vl *VictoriaLogs) Query(ctx context.Context, query string, start, end int64, limit int) ([]LogEntry, error) {
	return vl.QueryWithOffset(ctx, query, start, end, limit, 0)
}

func (vl *VictoriaLogs) QueryWithOffset(ctx context.Context, query string, start, end int64, limit, offset int) ([]LogEntry, error) {
	params := url.Values{}
	params.Set("query", query)
	addTimeRangeParams(params, start, end)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	} else {
		params.Set("limit", strconv.Itoa(vl.MaxQueryRows)) // 默认 1000 条
	}
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}

	endpoint := fmt.Sprintf("%s/select/logsql/query", vl.VictorialogsAddr)

	resp, err := vl.doRequest(ctx, "POST", endpoint, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// VictoriaLogs returns NDJSON format (one JSON object per line)
	// 这里手工按 \n 切分而不用 bufio.Scanner：Scanner 单行超过 64KB 就会返回
	// token too long 并中断整批解析，一条超长的 Java 堆栈会把同批其他日志一起带走。
	var logs []LogEntry
	for rest := body; len(rest) > 0; {
		var line []byte
		if idx := bytes.IndexByte(rest, '\n'); idx >= 0 {
			line, rest = rest[:idx], rest[idx+1:]
		} else {
			line, rest = rest, nil
		}
		line = bytes.TrimSpace(line) // 顺带吃掉 CRLF 的 \r
		if len(line) == 0 {
			continue
		}

		var entry LogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("decode log entry failed: %w, line=%s", err, errLineSnippet(line))
		}
		truncateLogEntry(entry)
		logs = append(logs, entry)
	}

	return logs, nil
}

// truncateLogEntry 把 entry 里超长的字符串字段截到 maxLogFieldSize 并附上尾注，
// 让使用者知道内容被截过、原始有多大。
// 只改写已有 key、不新增字段：range 期间新增的 key 是否被遍历没有保证，
// 而且多出来的字段会混进前端的日志字段列表里。
func truncateLogEntry(entry LogEntry) {
	for k, v := range entry {
		s, ok := v.(string)
		if !ok || len(s) <= maxLogFieldSize {
			continue
		}
		entry[k] = fmt.Sprintf("%s...(truncated, total %d bytes)", truncateUTF8(s, maxLogFieldSize), len(s))
	}
}

// truncateUTF8 按字节上限截断字符串，并回退到合法 UTF-8 边界，
// 避免切断多字节字符（如中文）导致前端显示成乱码。
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return trimPartialRune(s[:maxBytes])
}

// errLineSnippet 取坏行开头的一段用于报错。坏行本身可能有上百 MB，
// 先在 []byte 上切一刀再转 string，避免整行拷贝。
func errLineSnippet(line []byte) string {
	if len(line) <= maxErrLineSnippet {
		return string(line)
	}
	return trimPartialRune(string(line[:maxErrLineSnippet]))
}

// trimPartialRune 剥掉按字节切断后尾部残缺的多字节序列，最多回退 UTFMax-1 字节。
// 不用 utf8.ValidString 整段校验：未经 json 解码的原始响应字节中段就可能有非法字节，
// 那时「校验整段 + 逐字节回退」会退化成 O(n²)。
func trimPartialRune(s string) string {
	for i := 0; i < utf8.UTFMax-1 && len(s) > 0; i++ {
		// size <= 1 的 RuneError 才是非法编码，合法的 U+FFFD 自身占 3 字节
		if r, size := utf8.DecodeLastRuneInString(s); r == utf8.RuneError && size <= 1 {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}

// StatsQuery 执行统计查询（单点时间）
// POST /select/logsql/stats_query?query=<query>&time=<time>
func (vl *VictoriaLogs) StatsQuery(ctx context.Context, query string, time int64) (*PrometheusResponse, error) {
	params := url.Values{}
	params.Set("query", query)

	if time > 0 {
		params.Set("time", formatVictoriaLogsTimestamp(time))
	}

	endpoint := fmt.Sprintf("%s/select/logsql/stats_query", vl.VictorialogsAddr)

	resp, err := vl.doRequest(ctx, "POST", endpoint, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stats query failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result PrometheusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w, body=%s", err, string(body))
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("query failed: %s", result.Error)
	}

	return &result, nil
}

// StatsQueryRange 执行统计查询（时间范围）
// POST /select/logsql/stats_query_range?query=<query>&start=<start>&end=<end>&step=<step>
func (vl *VictoriaLogs) StatsQueryRange(ctx context.Context, query string, start, end int64, step string) (*PrometheusResponse, error) {
	params := url.Values{}
	params.Set("query", query)
	addTimeRangeParams(params, start, end)
	if step != "" {
		params.Set("step", step)
	}

	endpoint := fmt.Sprintf("%s/select/logsql/stats_query_range", vl.VictorialogsAddr)

	resp, err := vl.doRequest(ctx, "POST", endpoint, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stats query range failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result PrometheusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w, body=%s", err, string(body))
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("query failed: %s", result.Error)
	}

	return &result, nil
}

// HitsLogs 返回查询命中的日志数量，用于计算 total
// POST /select/logsql/hits?query=<query>&start=<start>&end=<end>&step=<step>
func (vl *VictoriaLogs) HitsLogs(ctx context.Context, query string, start, end int64) (int64, error) {
	step := ""
	startMs := normalizeVictoriaLogsTimestamp(start)
	endMs := normalizeVictoriaLogsTimestamp(end)
	if startMs > 0 && endMs > startMs {
		step = dskittypes.DefaultHistogramStepFromUnixRange(start, end)
	}

	result, err := vl.QueryHits(ctx, query, start, end, step)
	if err != nil {
		return 0, err
	}

	if len(result.Hits) == 0 {
		return 0, nil
	}

	return result.Hits[0].Total, nil
}

func (vl *VictoriaLogs) QueryHits(ctx context.Context, query string, start, end int64, step string, groupByFields ...string) (*HitsResult, error) {
	return vl.QueryHitsWithFieldsLimit(ctx, query, start, end, step, 5, groupByFields...)
}

func (vl *VictoriaLogs) QueryHitsWithFieldsLimit(ctx context.Context, query string, start, end int64, step string, fieldsLimit int, groupByFields ...string) (*HitsResult, error) {
	params := url.Values{}
	params.Set("query", query)
	addTimeRangeParams(params, start, end)
	if step != "" {
		params.Set("step", step)
	}
	if len(groupByFields) > 0 {
		hasField := false
		for _, field := range groupByFields {
			if field != "" {
				params.Add("field", field)
				hasField = true
			}
		}
		if hasField {
			if fieldsLimit <= 0 {
				fieldsLimit = 5
			}
			params.Set("fields_limit", strconv.Itoa(fieldsLimit))
		}
	}

	endpoint := fmt.Sprintf("%s/select/logsql/hits", vl.VictorialogsAddr)

	resp, err := vl.doRequest(ctx, "POST", endpoint, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hits query failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result HitsResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w, body=%s", err, string(body))
	}

	return &result, nil
}

func (vl *VictoriaLogs) StreamFieldNames(ctx context.Context, query string, start, end int64, filter string) ([]StreamFieldValue, error) {
	return vl.streamFieldValues(ctx, "stream_field_names", query, start, end, "", 0, filter)
}

func (vl *VictoriaLogs) StreamFieldValues(ctx context.Context, query string, start, end int64, field string, limit int, filter string) ([]StreamFieldValue, error) {
	return vl.streamFieldValues(ctx, "stream_field_values", query, start, end, field, limit, filter)
}

func (vl *VictoriaLogs) FieldNames(ctx context.Context, query string, start, end int64, limit int, filter string) ([]StreamFieldValue, error) {
	return vl.streamFieldValues(ctx, "field_names", query, start, end, "", limit, filter)
}

func (vl *VictoriaLogs) FieldValues(ctx context.Context, query string, start, end int64, field string, limit int, filter string) ([]StreamFieldValue, error) {
	return vl.streamFieldValues(ctx, "field_values", query, start, end, field, limit, filter)
}

func (vl *VictoriaLogs) streamFieldValues(ctx context.Context, path, query string, start, end int64, field string, limit int, filter string) ([]StreamFieldValue, error) {
	params := url.Values{}
	if query == "" {
		query = "*"
	}
	params.Set("query", query)
	params.Set("ignore_pipes", "1")
	addTimeRangeParams(params, start, end)
	if field != "" {
		params.Set("field", field)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if filter != "" {
		params.Set("filter", filter)
	}

	endpoint := fmt.Sprintf("%s/select/logsql/%s", vl.VictorialogsAddr, path)

	resp, err := vl.doRequest(ctx, "POST", endpoint, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s query failed: status=%d, body=%s", path, resp.StatusCode, string(body))
	}

	var result streamValuesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w, body=%s", err, string(body))
	}

	return result.Values, nil
}

// doRequest 执行 HTTP 请求
func (vl *VictoriaLogs) doRequest(ctx context.Context, method, endpoint string, params url.Values) (*http.Response, error) {
	var req *http.Request
	var err error

	if method == "GET" {
		fullURL := endpoint
		if len(params) > 0 {
			fullURL = fmt.Sprintf("%s?%s", endpoint, params.Encode())
		}
		req, err = http.NewRequestWithContext(ctx, method, fullURL, nil)
	} else {
		// POST with form data
		req, err = http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(params.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}

	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	if vl.VictorialogsBasic.VictorialogsUser != "" {
		req.SetBasicAuth(vl.VictorialogsBasic.VictorialogsUser, vl.VictorialogsBasic.VictorialogsPass)
	}

	// Custom Headers
	for k, v := range vl.Headers {
		req.Header.Set(k, v)
	}

	return vl.HTTPClient.Do(req)
}

func addTimeRangeParams(params url.Values, start, end int64) {
	if start > 0 {
		params.Set("start", formatVictoriaLogsTimestamp(start))
	}
	if end > 0 {
		params.Set("end", formatVictoriaLogsTimestamp(end))
	}
}

func formatVictoriaLogsTimestamp(value int64) string {
	if value <= 0 {
		return "0"
	}
	return strconv.FormatInt(normalizeVictoriaLogsTimestamp(value), 10)
}

func normalizeVictoriaLogsTimestamp(value int64) int64 {
	if value <= 0 {
		return value
	}
	return dskittypes.NormalizeUnixMillisecondsInt(value)
}
