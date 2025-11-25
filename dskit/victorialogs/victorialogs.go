package victorialogs

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type VictoriaLogsClient struct {
	Url                 string            `json:"victorialogs.url" mapstructure:"victorialogs.url"`
	Username            string            `json:"victorialogs.basic.username" mapstructure:"victorialogs.basic.username"`
	Password            string            `json:"victorialogs.basic.password" mapstructure:"victorialogs.basic.password"`
	SkipTLSVerify       bool              `json:"victorialogs.skip_tls_verify" mapstructure:"victorialogs.skip_tls_verify"`
	DialTimeout         int               `json:"victorialogs.dial_timeout" mapstructure:"victorialogs.dial_timeout"`
	MaxIdleConnsPerHost int               `json:"victorialogs.max_idle_conns_per_host" mapstructure:"victorialogs.max_idle_conns_per_host"`
	Headers             map[string]string `json:"victorialogs.headers" mapstructure:"victorialogs.headers"`
	Client              *http.Client      `json:"-" mapstructure:"-"`
}

type QueryParam struct {
	Query   string `json:"query" mapstructure:"query"`
	Limit   int    `json:"limit" mapstructure:"limit"`
	Start   int64  `json:"start" mapstructure:"start"`     // unix 秒
	End     int64  `json:"end" mapstructure:"end"`         // unix 秒
	Time    int64  `json:"time" mapstructure:"time"`       // unix 秒，相对于 now 的时间范围或者绝对时间
	Timeout int    `json:"timeout" mapstructure:"timeout"` // 超时时间，单位秒
	Ref     string `json:"ref" mapstructure:"ref"`         // 查询引用 ID
}

type HitsResult struct {
	Hits []struct {
		Total int64 `json:"total"`
	}
}

type StatsQueryRangeResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]interface{} `json:"metric"`
			Values [][]interface{}        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

type StatsQueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]interface{} `json:"metric"`
			Value  []interface{}          `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (q *QueryParam) MakeBody() url.Values {
	data := url.Values{}
	data.Set("query", q.Query)

	if q.Limit > 0 {
		data.Set("limit", fmt.Sprintf("%d", q.Limit))
	}

	if q.Time != 0 {
		data.Set("time", fmt.Sprintf("%d", q.Time*1000))
	} else {
		if q.Start != 0 {
			data.Set("start", fmt.Sprintf("%d", q.Start*1000))
		}
		if q.End != 0 {
			data.Set("end", fmt.Sprintf("%d", q.End*1000))
		}
	}

	if q.Timeout != 0 {
		data.Set("timeout", fmt.Sprintf("%ds", q.Timeout))
	}

	return data
}

// IsInstantQuery 判断是否为即时查询
func (q *QueryParam) IsInstantQuery() bool {
	return q.Time > 0 || (q.Start > 0 && q.Start == q.End) || q.Start == 0
}

type authTransport struct {
	base   http.RoundTripper
	header string // base64 encoded "user:pass"
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 不要直接修改传入的 req，因为可能被复用；克隆后修改 header
	if t.header != "" && req.Header.Get("Authorization") == "" {
		req2 := req.Clone(req.Context())
		req2.Header = req.Header.Clone()
		req2.Header.Set("Authorization", fmt.Sprintf("Basic %s", t.header))
		return t.base.RoundTrip(req2)
	}
	return t.base.RoundTrip(req)
}

func (v *VictoriaLogsClient) InitCli() error {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   time.Duration(v.DialTimeout) * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: v.SkipTLSVerify,
		},
		MaxIdleConnsPerHost: v.MaxIdleConnsPerHost,
	}

	var basic string
	if v.Username != "" {
		basic = base64.StdEncoding.EncodeToString([]byte(v.Username + ":" + v.Password))
		transport.ProxyConnectHeader = http.Header{
			"Authorization": []string{fmt.Sprintf("Basic %s", basic)},
		}
	}

	v.Client = &http.Client{
		Transport: &authTransport{
			base:   transport,
			header: basic,
		},
	}
	return nil
}

func (v *VictoriaLogsClient) QueryLogs(ctx context.Context, qp *QueryParam) ([]map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s%s", v.Url, "/select/logsql/query"), strings.NewReader(qp.MakeBody().Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	if v.Username != "" {
		req.SetBasicAuth(v.Username, v.Password)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	dec := json.NewDecoder(resp.Body)
	results := make([]map[string]interface{}, 0, 16)

	for {
		// respect context cancellation
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		// respect limit
		if qp.Limit > 0 && len(results) >= qp.Limit {
			return results, nil
		}

		var obj map[string]interface{}
		if err := dec.Decode(&obj); err != nil {
			if err == io.EOF {
				break
			}
			return results, fmt.Errorf("failed to decode stream: %v", err)
		}

		results = append(results, obj)
	}

	return results, nil
}

func (v *VictoriaLogsClient) StatsQueryRange(ctx context.Context, qp *QueryParam) (*StatsQueryRangeResult, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s%s", v.Url, "/select/logsql/stats_query_range"), strings.NewReader(qp.MakeBody().Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	if v.Username != "" {
		req.SetBasicAuth(v.Username, v.Password)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	dec := json.NewDecoder(resp.Body)
	var sqr StatsQueryRangeResult
	if err := dec.Decode(&sqr); err != nil {
		return nil, fmt.Errorf("failed to decode stats response: %v", err)
	}

	return &sqr, nil
}

func (v *VictoriaLogsClient) StatsQuery(ctx context.Context, qp *QueryParam) (*StatsQueryResult, error) {
	if qp.Time == 0 {
		qp.Time = time.Now().Unix()
	}
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s%s", v.Url, "/select/logsql/stats_query"), strings.NewReader(qp.MakeBody().Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	if v.Username != "" {
		req.SetBasicAuth(v.Username, v.Password)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	dec := json.NewDecoder(resp.Body)
	var sr StatsQueryResult
	if err := dec.Decode(&sr); err != nil {
		return nil, fmt.Errorf("failed to decode stats response: %v", err)
	}

	return &sr, nil
}

// HitsLogs 返回查询命中的日志数量，用于计算total
func (v *VictoriaLogsClient) HitsLogs(ctx context.Context, qp *QueryParam) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s%s", v.Url, "/select/logsql/hits"), strings.NewReader(qp.MakeBody().Encode()))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	if v.Username != "" {
		req.SetBasicAuth(v.Username, v.Password)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.Client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}
	dec := json.NewDecoder(resp.Body)
	var hr HitsResult
	if err := dec.Decode(&hr); err != nil {
		return 0, fmt.Errorf("failed to decode hits response: %v", err)
	}
	if len(hr.Hits) == 0 {
		return 0, nil
	}
	return hr.Hits[0].Total, nil
}
