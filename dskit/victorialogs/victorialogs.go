package victorialogs

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type VictoriaLogsClient struct {
	Url           string       `json:"vl.url" mapstructure:"vl.url"`
	User          string       `json:"vl.user" mapstructure:"vl.user"`
	Password      string       `json:"vl.password" mapstructure:"vl.password"`
	MaxQueryRows  int          `json:"vl.max_query_rows" mapstructure:"vl.max_query_rows"`
	SkipTLSVerify bool         `json:"vl.skip_tls_verify" mapstructure:"vl.skip_tls_verify"`
	Client        *http.Client `json:"-" mapstructure:"-"`
}

type QueryParam struct {
	Query   string `json:"query" mapstructure:"query"`
	Limit   int    `json:"limit" mapstructure:"limit"`
	Start   int64  `json:"start" mapstructure:"start"`     // unix 秒
	End     int64  `json:"end" mapstructure:"end"`         // unix 秒
	Time    int64  `json:"time" mapstructure:"time"`       // unix 秒，相对于 now 的时间范围或者绝对时间
	Timeout int    `json:"timeout" mapstructure:"timeout"` // 超时时间，单位秒
}

type HitsResult struct {
	Hits []struct {
		Total int64 `json:"total"`
	}
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

func (v *VictoriaLogsClient) InitCli() error {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: v.SkipTLSVerify,
		},
	}
	v.Client = &http.Client{
		Transport: transport,
	}
	data := url.Values{}
	data.Set("query", "*")
	data.Set("limit", "1")

	req, err := http.NewRequest("POST", v.Url+"/select/logsql/query", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	if v.User != "" {
		req.SetBasicAuth(v.User, v.Password)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// 使用 base client 的副本，设置探测超时
	c := *v.Client
	c.Timeout = 5 * time.Second

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (v *VictoriaLogsClient) Equal(other *VictoriaLogsClient) bool {
	return v.Url == other.Url &&
		v.User == other.User &&
		v.Password == other.Password &&
		v.MaxQueryRows == other.MaxQueryRows &&
		v.SkipTLSVerify == other.SkipTLSVerify
}

func (v *VictoriaLogsClient) Validate() error {
	return v.InitCli()
}

func (v *VictoriaLogsClient) QueryLogs(ctx context.Context, qp *QueryParam) ([]map[string]interface{}, error) {
	// 确保已经初始化 client
	if v.Client == nil {
		return nil, fmt.Errorf("http client is not initialized")
	}

	client := *v.Client
	client.Timeout = time.Duration(qp.Timeout) * time.Second
	req, err := http.NewRequestWithContext(ctx, "POST", v.Url+"/select/logsql/query", strings.NewReader(qp.MakeBody().Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	if v.User != "" {
		req.SetBasicAuth(v.User, v.Password)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
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

// HitsLogs 返回查询命中的日志数量，用于计算total
func (v *VictoriaLogsClient) HitsLogs(ctx context.Context, qp *QueryParam) (int64, error) {
	// 确保已经初始化 client
	if v.Client == nil {
		return 0, fmt.Errorf("http client is not initialized")
	}

	client := *v.Client
	client.Timeout = time.Duration(qp.Timeout) * time.Second
	req, err := http.NewRequestWithContext(ctx, "POST", v.Url+"/select/logsql/hits", strings.NewReader(qp.MakeBody().Encode()))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	if v.User != "" {
		req.SetBasicAuth(v.User, v.Password)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
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
