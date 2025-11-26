package victorialogs

import (
	"bufio"
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
)

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
	WriteAddr    string            `json:"victorialogs.write_addr" mapstructure:"victorialogs.write_addr"`
	TsdbType     string            `json:"victorialogs.tsdb_type" mapstructure:"victorialogs.tsdb_type"`
	InternalAddr string            `json:"victorialogs.internal_addr" mapstructure:"victorialogs.internal_addr"`

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
	Hits []struct {
		Total int64 `json:"total"`
	}
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
	params := url.Values{}
	params.Set("query", query)

	if start > 0 {
		params.Set("start", strconv.FormatInt(start, 10))
	}
	if end > 0 {
		params.Set("end", strconv.FormatInt(end, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	} else {
		params.Set("limit", "1000") // 默认 1000 条
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
	var logs []LogEntry
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("decode log entry failed: %w, line=%s", err, line)
		}
		logs = append(logs, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan response failed: %w", err)
	}

	return logs, nil
}

// StatsQuery 执行统计查询（单点时间）
// POST /select/logsql/stats_query?query=<query>&time=<time>
func (vl *VictoriaLogs) StatsQuery(ctx context.Context, query string, time int64) (*PrometheusResponse, error) {
	params := url.Values{}
	params.Set("query", query)

	if time > 0 {
		params.Set("time", strconv.FormatInt(time, 10))
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

	if start > 0 {
		params.Set("start", strconv.FormatInt(start, 10))
	}
	if end > 0 {
		params.Set("end", strconv.FormatInt(end, 10))
	}
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
// POST /select/logsql/hits?query=<query>&start=<start>&end=<end>
func (vl *VictoriaLogs) HitsLogs(ctx context.Context, query string, start, end int64) (int64, error) {
	params := url.Values{}
	params.Set("query", query)

	if start > 0 {
		params.Set("start", strconv.FormatInt(start, 10))
	}
	if end > 0 {
		params.Set("end", strconv.FormatInt(end, 10))
	}

	endpoint := fmt.Sprintf("%s/select/logsql/hits", vl.VictorialogsAddr)

	resp, err := vl.doRequest(ctx, "POST", endpoint, params)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read response body failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("hits query failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result HitsResult
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("decode response failed: %w, body=%s", err, string(body))
	}

	if len(result.Hits) == 0 {
		return 0, nil
	}

	return result.Hits[0].Total, nil
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
