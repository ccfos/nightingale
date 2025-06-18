package victorialogs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"
	"github.com/prometheus/common/model"

	"github.com/mitchellh/mapstructure"
	"github.com/toolkits/pkg/logger"
)

const (
	VictorialogsType = "victorialogs"
)

func init() {
	datasource.RegisterDatasource(VictorialogsType, new(Victorialogs))
}

type Victorialogs struct {
	Addr     string `json:"addr" mapstructure:"addr"`
	TLS      TLS    `json:"tls" mapstructure:"tls"`
	MaxLines int    `json:"max_lines" mapstructure:"max_lines"`
	Client   *http.Client
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
	newest.Addr = strings.TrimSuffix(newest.Addr, "/")
	return newest, err
}

func (v *Victorialogs) InitClient() error {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	if strings.Contains(v.Addr, "https") {
		tlsConfig := tlsx.ClientConfig{
			InsecureSkipVerify: v.TLS.SkipTlsVerify,
			UseTLS:             true,
		}
		cfg, err := tlsConfig.TLSConfig()
		if err != nil {
			return err
		}
		transport.TLSClientConfig = cfg
	}

	v.Client = &http.Client{
		Transport: transport,
	}
	return nil
}

func (v *Victorialogs) Validate(ctx context.Context) error {
	baseURL, err := url.Parse(v.Addr)
	if err != nil {
		return err
	}
	baseURL.Path = "/health"

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return err
	}

	resp, err := v.Client.Do(r)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Errorf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got status code: %d, expected: %d", resp.StatusCode, http.StatusOK)
	}

	return nil
}

func (v *Victorialogs) Equal(p datasource.Datasource) bool {
	newest, ok := p.(*Victorialogs)
	if !ok {
		logger.Errorf("unexpected plugin type, expected is victorialogs")
		return false
	}
	if v.Addr != newest.Addr {
		return false
	}
	if v.TLS.SkipTlsVerify != newest.TLS.SkipTlsVerify {
		return false
	}
	return true
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
		return nil, errors.New("invalid query param")
	}

	baseURL, err := url.Parse(v.Addr)
	if err != nil {
		return nil, err
	}
	baseURL.Path = "/select/logsql/stats_query_range"

	values := baseURL.Query()
	values.Add("query", queryParam.Query)
	values.Add("start", strconv.FormatInt(queryParam.Start, 10))
	values.Add("end", strconv.FormatInt(queryParam.End, 10))
	values.Add("step", queryParam.Step)
	baseURL.RawQuery = values.Encode()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := v.Client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var respData struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Values [][]interface{}   `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &respData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	if respData.Status != "success" {
		return nil, fmt.Errorf("query failed with status: %s", respData.Status)
	}

	var dataResps []models.DataResp
	for _, result := range respData.Data.Result {
		metric := make(model.Metric)
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

		dataResps = append(dataResps, models.DataResp{
			Ref:    queryParam.Ref,
			Metric: metric,
			Values: values,
			Query:  queryParam.Query,
		})
	}

	return dataResps, nil
}

func (v *Victorialogs) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	queryParam, ok := query.(*QueryParam)
	if !ok {
		return nil, 0, errors.New("invalid query param")
	}

	baseURL, err := url.Parse(v.Addr)
	if err != nil {
		return nil, 0, err
	}
	baseURL.Path = "/select/logsql/query"

	values := baseURL.Query()
	values.Add("query", queryParam.Query)
	values.Add("start", strconv.FormatInt(queryParam.Start, 10))
	values.Add("end", strconv.FormatInt(queryParam.End, 10))
	values.Add("limit", strconv.Itoa(v.MaxLines))
	baseURL.RawQuery = values.Encode()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := v.Client.Do(r)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// parse response - stream of JSON lines
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	// Split the response into lines and parse each line as JSON
	lines := strings.Split(string(body), "\n")
	var logs []interface{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse each line as a JSON object
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			logger.Warningf("failed to parse log line: %v, line: %s", err, line)
			continue
		}

		logs = append(logs, logEntry)
	}

	return logs, CalcHits(ctx, query, v), nil
}

func CalcHits(ctx context.Context, query interface{}, v *Victorialogs) int64 {
	queryParam, ok := query.(*QueryParam)
	if !ok {
		return 0
	}

	baseURL, err := url.Parse(v.Addr)
	if err != nil {
		return 0
	}
	baseURL.Path = "/select/logsql/hits"

	values := baseURL.Query()
	values.Add("query", queryParam.Query)
	values.Add("start", strconv.FormatInt(queryParam.Start, 10))
	values.Add("end", strconv.FormatInt(queryParam.End, 10))
	baseURL.RawQuery = values.Encode()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return 0
	}

	resp, err := v.Client.Do(r)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	var respData struct {
		Hits []struct {
			Total int64 `json:"total"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(body, &respData); err != nil {
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
