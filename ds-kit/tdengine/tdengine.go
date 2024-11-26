package tdengine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/ccfos/nightingale/v6/ds-kit/types"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"
	"github.com/mitchellh/mapstructure"
	"github.com/toolkits/pkg/logger"
	"net"
	"net/http"
	"strings"
	"time"
)

type Tdengine struct {
	DatasourceName string `json:"td.datasource_name" mapstructure:"td.datasource_name"`
	Url            string `json:"td.url" mapstructure:"td.url"`
	BasicAuthUser  string `json:"td.basic_auth_user" mapstructure:"td.basic_auth_user"`
	BasicAuthPass  string `json:"td.basic_auth_pass" mapstructure:"td.basic_auth_pass"`
	Token          string `json:"td.token" mapstructure:"td.token"`

	Timeout     int64 `json:"td.timeout" mapstructure:"td.timeout"`
	DialTimeout int64 `json:"td.dial_timeout" mapstructure:"td.dial_timeout"`

	MaxIdleConnsPerHost int `json:"td.max_idle_conns_per_host" mapstructure:"td.max_idle_conns_per_host"`

	Headers []string `json:"td.headers" mapstructure:"td.headers"`

	tlsx.ClientConfig

	header map[string][]string `json:"-"`
	client *http.Client        `json:"-"`
}

type APIResponse struct {
	Code       int             `json:"code"`
	ColumnMeta [][]interface{} `json:"column_meta"`
	Data       [][]interface{} `json:"data"`
	Rows       int             `json:"rows"`
}

type QueryParam struct {
	Database string `json:"database"`
	Table    string `json:"table"`
}

func (tc *Tdengine) InitCli() {

	tc.client = &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableCompression:    true,
		},
	}

	tc.header = map[string][]string{
		"Connection": {"keep-alive"},
	}

	for _, v := range tc.Headers {
		kv := strings.Split(v, ":")
		if len(kv) != 2 {
			continue
		}
		tc.header[kv[0]] = []string{kv[1]}
	}

	if tc.BasicAuthUser != "" {
		basic := base64.StdEncoding.EncodeToString([]byte(tc.BasicAuthUser + ":" + tc.BasicAuthPass))
		tc.header["Authorization"] = []string{fmt.Sprintf("Basic %s", basic)}
	}
}

func (tc *Tdengine) QueryTable(query string) (APIResponse, error) {
	var apiResp APIResponse
	req, err := http.NewRequest("POST", tc.Url+"/rest/sql", strings.NewReader(query))
	if err != nil {
		return apiResp, err
	}

	for k, v := range tc.header {
		req.Header[k] = v
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := tc.client.Do(req)
	if err != nil {
		return apiResp, err
	}
	defer resp.Body.Close()

	// 限制响应体大小为10MB
	maxSize := int64(10 * 1024 * 1024) // 10MB
	limitedReader := http.MaxBytesReader(nil, resp.Body, maxSize)

	if resp.StatusCode != http.StatusOK {
		return apiResp, fmt.Errorf("HTTP error, status: %s", resp.Status)
	}

	err = json.NewDecoder(limitedReader).Decode(&apiResp)
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			return apiResp, fmt.Errorf("response body exceeds 10MB limit")
		}
		return apiResp, err
	}

	return apiResp, nil
}

func (tc *Tdengine) ShowDatabases(context.Context) ([]string, error) {
	var databases []string
	data, err := tc.QueryTable("show databases")
	if err != nil {
		return databases, err
	}

	for _, row := range data.Data {
		databases = append(databases, row[0].(string))
	}
	return databases, nil
}

func (tc *Tdengine) ShowTables(ctx context.Context, database string) ([]string, error) {
	var tables []string
	sql := fmt.Sprintf("show %s.tables", database)
	//if isStable {
	//	sql = fmt.Sprintf("show %s.stables", database)
	//}

	data, err := tc.QueryTable(sql)
	if err != nil {
		return tables, err
	}

	for _, row := range data.Data {
		tables = append(tables, row[0].(string))
	}
	return tables, nil
}

func (tc *Tdengine) DescribeTable(ctx context.Context, query interface{}) ([]*types.ColumnProperty, error) {
	var columns []*types.ColumnProperty
	tdQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, tdQueryParam); err != nil {
		return nil, err
	}
	sql := fmt.Sprintf("select * from %s.%s limit 1", tdQueryParam.Database, tdQueryParam.Table)
	data, err := tc.QueryTable(sql)
	if err != nil {
		return columns, err
	}
	for _, row := range data.ColumnMeta {
		var colType string
		switch t := row[1].(type) {
		case float64:
			// v2版本数字类型映射
			switch int(t) {
			case 1:
				colType = "BOOL"
			case 2:
				colType = "TINYINT"
			case 3:
				colType = "SMALLINT"
			case 4:
				colType = "INT"
			case 5:
				colType = "BIGINT"
			case 6:
				colType = "FLOAT"
			case 7:
				colType = "DOUBLE"
			case 8:
				colType = "BINARY"
			case 9:
				colType = "TIMESTAMP"
			case 10:
				colType = "NCHAR"
			default:
				colType = "UNKNOWN"
			}
		case string:
			// v3版本直接使用字符串类型
			colType = t
		default:
			logger.Warningf("unexpected column type format: %v", row[1])
			colType = "UNKNOWN"
		}

		column := &types.ColumnProperty{
			Field: row[0].(string),
			Type:  colType,
		}
		columns = append(columns, column)
	}

	return columns, nil
}
