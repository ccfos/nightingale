package victorialogs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"

	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
)

const (
	VictorialogsType  = "victorialogs"
	DefaultMaxLines   = 1000
	DefaultTimeout    = 30
	MaxErrorBodySize  = 1024 * 1024 // 1MB
	DefaultBufferSize = 64 * 1024   // 64KB
)

func init() {
	datasource.RegisterDatasource(VictorialogsType, new(Victorialogs))
}

type Victorialogs struct {
	Addr     string            `json:"victorialogs.addr" mapstructure:"victorialogs.addr"`
	Timeout  int64             `json:"victorialogs.timeout" mapstructure:"victorialogs.timeout"`
	Basic    BasicAuth         `json:"victorialogs.basic" mapstructure:"victorialogs.basic"`
	TLS      TLS               `json:"victorialogs.tls" mapstructure:"victorialogs.tls"`
	Headers  map[string]string `json:"victorialogs.headers" mapstructure:"victorialogs.headers"`
	MaxLines int               `json:"victorialogs.max_lines" mapstructure:"victorialogs.max_lines"`
	Client   *http.Client
	once     sync.Once
}

type BasicAuth struct {
	Username string `json:"victorialogs.user" mapstructure:"victorialogs.user"`
	Password string `json:"victorialogs.password" mapstructure:"victorialogs.password"`
}

type TLS struct {
	SkipTlsVerify bool `json:"skip_tls_verify" mapstructure:"skip_tls_verify"`
}

type QueryParam struct {
	Ref   string `json:"ref" mapstructure:"ref"`
	Query string `json:"query" mapstructure:"query"`
	Start int64  `json:"start" mapstructure:"start"`
	End   int64  `json:"end" mapstructure:"end"`
	Step  string `json:"step" mapstructure:"step"`
}

func (v *Victorialogs) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(Victorialogs)
	err := mapstructure.Decode(settings, newest)
	if err != nil {
		return nil, err
	}

	// 设置默认值
	newest.Addr = strings.TrimSuffix(newest.Addr, "/")
	if newest.Timeout <= 0 {
		newest.Timeout = DefaultTimeout
	}
	if newest.MaxLines <= 0 {
		newest.MaxLines = DefaultMaxLines
	}
	newest.InitClient()
	return newest, nil
}

func (v *Victorialogs) InitClient() error {
	var initErr error
	v.once.Do(func() {
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}

		if strings.HasPrefix(v.Addr, "https://") {
			tlsConfig := tlsx.ClientConfig{
				InsecureSkipVerify: v.TLS.SkipTlsVerify,
				UseTLS:             true,
			}
			cfg, err := tlsConfig.TLSConfig()
			if err != nil {
				initErr = fmt.Errorf("failed to create TLS config: %w", err)
				return
			}
			transport.TLSClientConfig = cfg
		}

		v.Client = &http.Client{
			Transport: transport,
			Timeout:   time.Duration(v.Timeout) * time.Second,
		}
	})
	return initErr
}

// buildURL 统一URL构建逻辑，减少重复代码
func (v *Victorialogs) buildURL(path string, queryParam *QueryParam, extraParams map[string]string) (*url.URL, error) {
	baseURL, err := url.Parse(v.Addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}
	baseURL.Path = path

	values := baseURL.Query()
	if queryParam != nil {
		if queryParam.Query != "" {
			values.Add("query", queryParam.Query)
		}
		if queryParam.Start != 0 {
			values.Add("start", strconv.FormatInt(queryParam.Start, 10))
		}
		if queryParam.End != 0 {
			values.Add("end", strconv.FormatInt(queryParam.End, 10))
		}
		if queryParam.Step != "" {
			values.Add("step", queryParam.Step)
		}
	}

	for k, v := range extraParams {
		values.Add(k, v)
	}

	baseURL.RawQuery = values.Encode()
	return baseURL, nil
}

// createRequest 统一请求创建逻辑
func (v *Victorialogs) createRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	v.AuthAndHeaders(req)
	return req, nil
}

// readLimitedErrorBody 统一错误响应体读取
func readLimitedErrorBody(resp *http.Response) string {
	limitedReader := io.LimitReader(resp.Body, MaxErrorBodySize)
	body, _ := io.ReadAll(limitedReader)
	return string(body)
}

func (v *Victorialogs) Validate(ctx context.Context) error {
	url, err := v.buildURL("/health", nil, nil)
	if err != nil {
		return err
	}

	req, err := v.createRequest(ctx, url.String())
	if err != nil {
		return err
	}

	resp, err := v.Client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Errorf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body := readLimitedErrorBody(resp)
		return fmt.Errorf("health check failed with status %d: %s", resp.StatusCode, body)
	}

	return nil
}

func (v *Victorialogs) Equal(p datasource.Datasource) bool {
	newest, ok := p.(*Victorialogs)
	if !ok {
		logger.Errorf("unexpected plugin type, expected is victorialogs")
		return false
	}
	return v.Addr == newest.Addr &&
		v.Timeout == newest.Timeout &&
		v.TLS.SkipTlsVerify == newest.TLS.SkipTlsVerify &&
		v.Basic.Username == newest.Basic.Username
}

func (v *Victorialogs) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (v *Victorialogs) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (v *Victorialogs) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	queryParam, ok := query.(*QueryParam)
	if !ok {
		return nil, errors.New("invalid query param: expected *QueryParam")
	}

	if queryParam.Query == "" {
		return nil, errors.New("query cannot be empty")
	}

	url, err := v.buildURL("/select/logsql/stats_query_range", queryParam, nil)
	if err != nil {
		return nil, err
	}

	req, err := v.createRequest(ctx, url.String())
	if err != nil {
		return nil, err
	}

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := readLimitedErrorBody(resp)
		return nil, fmt.Errorf("query failed with status %d: %s", resp.StatusCode, body)
	}

	decoder := json.NewDecoder(resp.Body)

	var respData struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Values [][]interface{}   `json:"values"`
			} `json:"result"`
		} `json:"data"`
		Error     string `json:"error,omitempty"`
		ErrorType string `json:"errorType,omitempty"`
	}

	if err := decoder.Decode(&respData); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if respData.Status != "success" {
		if respData.Error != "" {
			return nil, fmt.Errorf("query failed: %s (%s)", respData.Error, respData.ErrorType)
		}
		return nil, fmt.Errorf("query failed with status: %s", respData.Status)
	}

	// 预分配切片容量
	dataResps := make([]models.DataResp, 0, len(respData.Data.Result))
	for _, result := range respData.Data.Result {
		metric := make(model.Metric, len(result.Metric))
		for k, v := range result.Metric {
			metric[model.LabelName(k)] = model.LabelValue(v)
		}

		values := make([][]float64, 0, len(result.Values))
		for _, v := range result.Values {
			if len(v) != 2 {
				continue
			}

			ts, ok := v[0].(float64)
			if !ok {
				continue
			}

			var val float64
			switch v1 := v[1].(type) {
			case float64:
				val = v1
			case string:
				if parsed, err := strconv.ParseFloat(v1, 64); err == nil {
					val = parsed
				} else {
					continue
				}
			default:
				continue
			}

			values = append(values, []float64{ts, val})
		}

		if len(values) > 0 {
			dataResps = append(dataResps, models.DataResp{
				Ref:    queryParam.Ref,
				Metric: metric,
				Values: values,
				Query:  queryParam.Query,
			})
		}
	}

	return dataResps, nil
}

func (v *Victorialogs) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	queryParam, ok := query.(*QueryParam)
	if !ok {
		return nil, 0, errors.New("invalid query param: expected *QueryParam")
	}

	if queryParam.Query == "" {
		return nil, 0, errors.New("query cannot be empty")
	}

	extraParams := map[string]string{
		"limit": strconv.Itoa(v.MaxLines),
	}
	url, err := v.buildURL("/select/logsql/query", queryParam, extraParams)
	if err != nil {
		return nil, 0, err
	}

	req, err := v.createRequest(ctx, url.String())
	if err != nil {
		return nil, 0, err
	}

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("query failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := readLimitedErrorBody(resp)
		return nil, 0, fmt.Errorf("query failed with status %d: %s", resp.StatusCode, body)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, DefaultBufferSize), 0) // 0 表示无限制

	logs := make([]interface{}, 0, v.MaxLines)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			logger.Warningf("failed to parse log line: %v, line: %s", err, line)
			continue
		}

		logs = append(logs, logEntry)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("error reading response: %w", err)
	}

	var hits int64
	if len(logs) > 0 {
		hits = v.calcHits(ctx, queryParam)
	}

	return logs, hits, nil
}

func (v *Victorialogs) calcHits(ctx context.Context, queryParam *QueryParam) int64 {
	url, err := v.buildURL("/select/logsql/hits", queryParam, nil)
	if err != nil {
		logger.Errorf("invalid address for hits calculation: %v", err)
		return 0
	}

	req, err := v.createRequest(ctx, url.String())
	if err != nil {
		logger.Errorf("failed to create hits request: %v", err)
		return 0
	}

	resp, err := v.Client.Do(req)
	if err != nil {
		logger.Errorf("hits query failed: %v", err)
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Errorf("hits query failed with status: %d", resp.StatusCode)
		return 0
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Errorf("failed to read hits response: %v", err)
		return 0
	}

	var respData struct {
		Hits []struct {
			Total int64 `json:"total"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(body, &respData); err != nil {
		logger.Errorf("failed to unmarshal hits response: %v", err)
		return 0
	}

	var total int64
	for _, hit := range respData.Hits {
		total += hit.Total
	}

	return total
}

func (v *Victorialogs) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (v *Victorialogs) AuthAndHeaders(req *http.Request) {
	for k, v := range v.Headers {
		req.Header.Set(k, v)
	}

	if v.Basic.Username != "" {
		req.SetBasicAuth(v.Basic.Username, v.Basic.Password)
	}
}
