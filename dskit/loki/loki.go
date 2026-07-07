package loki

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type LokiBasicAuth struct {
	LokiUser      string `json:"loki.user" mapstructure:"loki.user"`
	LokiPass      string `json:"loki.password" mapstructure:"loki.password"`
	LokiIsEncrypt bool   `json:"loki.is_encrypt" mapstructure:"loki.is_encrypt"`
}

type LokiTLS struct {
	SkipTlsVerify bool `json:"loki.tls.skip_tls_verify" mapstructure:"loki.tls.skip_tls_verify"`
}

type Loki struct {
	LokiAddr  string        `json:"loki.addr" mapstructure:"loki.addr"`
	LokiBasic LokiBasicAuth `json:"loki.basic" mapstructure:"loki.basic"`
	LokiTls   LokiTLS       `json:"loki.tls" mapstructure:"loki.tls"`

	Headers             map[string]string `json:"loki.headers" mapstructure:"loki.headers"`
	Timeout             int64             `json:"loki.timeout" mapstructure:"loki.timeout"`
	DialTimeout         int64             `json:"loki.dial_timeout" mapstructure:"loki.dial_timeout"`
	MaxIdleConnsPerHost int               `json:"loki.max_idle_conns_per_host" mapstructure:"loki.max_idle_conns_per_host"`
	ClusterName         string            `json:"loki.cluster_name" mapstructure:"loki.cluster_name"`
	MaxQueryRows        int               `json:"loki.max_query_rows" mapstructure:"loki.max_query_rows"`
	HTTPClient          *http.Client      `json:"-" mapstructure:"-"`
}

type QueryResponse struct {
	Status string    `json:"status"`
	Data   QueryData `json:"data"`
	Error  string    `json:"error,omitempty"`
}

type QueryData struct {
	ResultType string      `json:"resultType"`
	Result     []QueryItem `json:"result"`
}

type QueryItem struct {
	Stream map[string]string `json:"stream,omitempty"`
	Metric map[string]string `json:"metric,omitempty"`
	Value  []interface{}     `json:"value,omitempty"`
	Values [][]interface{}   `json:"values,omitempty"`
}

type responseEnvelope struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type LabelValue struct {
	Value string `json:"value"`
}

type FieldMeta struct {
	Field        string   `json:"field"`
	InferredType string   `json:"inferred_type,omitempty"`
	Values       []string `json:"values,omitempty"`
}

type NormalizedLog map[string]interface{}

const (
	parsedFieldsDefaultLimit = 200
	parsedFieldsMaxLimit     = 500
	parsedFieldValuesMax     = 100
	parsedFieldsTimeout      = 5 * time.Second
)

func (vl *Loki) InitHTTPClient() error {
	maxIdleConnsPerHost := vl.MaxIdleConnsPerHost
	if maxIdleConnsPerHost <= 0 {
		maxIdleConnsPerHost = 10
	}

	dialTimeout := time.Duration(vl.DialTimeout) * time.Millisecond
	if dialTimeout == 0 {
		dialTimeout = 30 * time.Second
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: vl.LokiTls.SkipTlsVerify,
		},
	}
	transport.DialContext = (&netDialer{timeout: dialTimeout}).DialContext

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

type netDialer struct {
	timeout time.Duration
}

func (d *netDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: d.timeout}
	return dialer.DialContext(ctx, network, address)
}

func (vl *Loki) LabelNames(ctx context.Context, query string, start, end int64, filter string, limit int) ([]string, error) {
	params := url.Values{}
	if query != "" {
		params.Set("query", query)
	}
	addTimeRangeParams(params, start, end)

	body, err := vl.doRequest(ctx, http.MethodGet, "/api/v1/labels", params)
	if err != nil {
		return nil, err
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode labels response failed: %w, body=%s", err, string(body))
	}
	if envelope.Status != "success" {
		return nil, fmt.Errorf("labels query failed: %s", envelope.Error)
	}

	var values []string
	if err := json.Unmarshal(envelope.Data, &values); err != nil {
		return nil, fmt.Errorf("decode labels data failed: %w", err)
	}
	return filterStrings(values, filter, limit), nil
}

func (vl *Loki) LabelValues(ctx context.Context, query string, start, end int64, label, filter string, limit int) ([]LabelValue, error) {
	params := url.Values{}
	if query != "" {
		params.Set("query", query)
	}
	addTimeRangeParams(params, start, end)

	body, err := vl.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/label/%s/values", url.PathEscape(label)), params)
	if err != nil {
		return nil, err
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode label values response failed: %w, body=%s", err, string(body))
	}
	if envelope.Status != "success" {
		return nil, fmt.Errorf("label values query failed: %s", envelope.Error)
	}

	var values []string
	if err := json.Unmarshal(envelope.Data, &values); err != nil {
		return nil, fmt.Errorf("decode label values data failed: %w", err)
	}

	values = filterStrings(values, filter, limit)
	ret := make([]LabelValue, 0, len(values))
	for _, value := range values {
		ret = append(ret, LabelValue{Value: value})
	}
	return ret, nil
}

func (vl *Loki) QueryRange(ctx context.Context, query string, start, end int64, step string, limit int, direction string) (*QueryResponse, error) {
	params := url.Values{}
	params.Set("query", query)
	addTimeRangeParams(params, start, end)
	if step != "" {
		params.Set("step", step)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if direction != "" {
		params.Set("direction", direction)
	}

	body, err := vl.doRequest(ctx, http.MethodGet, "/api/v1/query_range", params)
	if err != nil {
		return nil, err
	}

	var result QueryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode query_range response failed: %w, body=%s", err, string(body))
	}
	if result.Status != "success" {
		return nil, fmt.Errorf("query_range failed: %s", result.Error)
	}
	return &result, nil
}

func (vl *Loki) QueryInstant(ctx context.Context, query string, ts int64) (*QueryResponse, error) {
	params := url.Values{}
	params.Set("query", query)
	if ts > 0 {
		params.Set("time", formatLokiTimestamp(ts))
	}

	body, err := vl.doRequest(ctx, http.MethodGet, "/api/v1/query", params)
	if err != nil {
		return nil, err
	}

	var result QueryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode query response failed: %w, body=%s", err, string(body))
	}
	if result.Status != "success" {
		return nil, fmt.Errorf("query failed: %s", result.Error)
	}
	return &result, nil
}

func (vl *Loki) ParsedFields(ctx context.Context, query string, start, end int64, limit int) ([]FieldMeta, error) {
	if limit <= 0 {
		limit = parsedFieldsDefaultLimit
	}
	if limit > parsedFieldsMaxLimit {
		limit = parsedFieldsMaxLimit
	}

	queryCtx, cancel := context.WithTimeout(ctx, parsedFieldsTimeout)
	defer cancel()

	result, err := vl.QueryRange(queryCtx, query, start, end, "", limit, "backward")
	if err != nil {
		return nil, err
	}

	fieldValues := make(map[string]map[string]struct{})
	for _, item := range result.Data.Result {
		for key, value := range item.Stream {
			if key == "" {
				continue
			}
			if _, exists := fieldValues[key]; !exists {
				fieldValues[key] = make(map[string]struct{})
			}
			if value != "" && len(fieldValues[key]) < parsedFieldValuesMax {
				fieldValues[key][value] = struct{}{}
			}
		}
	}

	keys := make([]string, 0, len(fieldValues))
	for key := range fieldValues {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	ret := make([]FieldMeta, 0, len(keys))
	for _, key := range keys {
		values := make([]string, 0, len(fieldValues[key]))
		for value := range fieldValues[key] {
			values = append(values, value)
		}
		sort.Strings(values)
		ret = append(ret, FieldMeta{
			Field:  key,
			Values: values,
		})
	}
	return ret, nil
}

func NormalizeLogs(resp *QueryResponse) []NormalizedLog {
	if resp == nil {
		return nil
	}

	ret := make([]NormalizedLog, 0)
	for _, item := range resp.Data.Result {
		for _, value := range item.Values {
			if len(value) < 2 {
				continue
			}
			tsNs, ok := ParseTimestampNanos(value[0])
			if !ok {
				continue
			}
			line := fmt.Sprintf("%v", value[1])
			log := NormalizedLog{
				"timestamp":     tsNs / int64(time.Millisecond),
				"__timestamp__": strconv.FormatInt(tsNs, 10),
				"line":          line,
				"stream":        item.Stream,
			}
			ret = append(ret, log)
		}
	}
	return ret
}

func (vl *Loki) apiURL(path string) (string, error) {
	u, err := url.Parse(vl.LokiAddr)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimRight(u.Path, "/")
	apiPath := "/" + strings.TrimLeft(path, "/")
	u.Path = basePath + apiPath
	return u.String(), nil
}

func (vl *Loki) doRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
	if vl.HTTPClient == nil {
		if err := vl.InitHTTPClient(); err != nil {
			return nil, err
		}
	}

	endpoint, err := vl.apiURL(path)
	if err != nil {
		return nil, fmt.Errorf("invalid loki addr: %w", err)
	}

	if method == http.MethodGet && len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	var body io.Reader
	if method != http.MethodGet {
		body = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if vl.LokiBasic.LokiUser != "" {
		req.SetBasicAuth(vl.LokiBasic.LokiUser, vl.LokiBasic.LokiPass)
	}
	for k, v := range vl.Headers {
		req.Header.Set(k, v)
	}

	resp, err := vl.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("loki request failed: path=%s status=%d body=%s", path, resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func addTimeRangeParams(params url.Values, start, end int64) {
	if start > 0 {
		params.Set("start", formatLokiTimestamp(start))
	}
	if end > 0 {
		params.Set("end", formatLokiTimestamp(end))
	}
}

func formatLokiTimestamp(value int64) string {
	switch {
	case value <= 0:
		return "0"
	case value < 1e12:
		return strconv.FormatInt(value*int64(time.Second), 10)
	case value < 1e15:
		return strconv.FormatInt(value*int64(time.Millisecond), 10)
	case value < 1e18:
		return strconv.FormatInt(value*int64(time.Microsecond), 10)
	default:
		return strconv.FormatInt(value, 10)
	}
}

func ParseTimestampNanos(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return normalizeUnixTimestampNanosFloat(v), true
	case int64:
		return normalizeUnixTimestampNanosInt(v), true
	case int:
		return normalizeUnixTimestampNanosInt(int64(v)), true
	case string:
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return normalizeUnixTimestampNanosInt(n), true
		}
		if strings.Contains(v, ".") {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return normalizeUnixTimestampNanosFloat(f), true
			}
		}
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t.UnixNano(), true
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t.UnixNano(), true
		}
	}
	return 0, false
}

func normalizeUnixTimestampNanosInt(value int64) int64 {
	switch {
	case value > 1e17:
		return value
	case value > 1e14:
		return value * int64(time.Microsecond)
	case value > 1e11:
		return value * int64(time.Millisecond)
	default:
		return value * int64(time.Second)
	}
}

func normalizeUnixTimestampNanosFloat(value float64) int64 {
	switch {
	case value > 1e17:
		return int64(value)
	case value > 1e14:
		return int64(value * 1e3)
	case value > 1e11:
		return int64(value * 1e6)
	default:
		return int64(value * 1e9)
	}
}

func filterStrings(values []string, filter string, limit int) []string {
	sort.Strings(values)
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if filter != "" && !strings.Contains(value, filter) {
			continue
		}
		ret = append(ret, value)
		if limit > 0 && len(ret) >= limit {
			break
		}
	}
	return ret
}
