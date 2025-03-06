package tdengine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"

	"github.com/toolkits/pkg/logger"
)

type Tdengine struct {
	Addr                string             `json:"tdengine.addr" mapstructure:"tdengine.addr"`
	Basic               *TDengineBasicAuth `json:"tdengine.basic" mapstructure:"tdengine.basic"`
	Token               string             `json:"tdengine.token" mapstructure:"tdengine.token"`
	Timeout             int64              `json:"tdengine.timeout" mapstructure:"tdengine.timeout"`
	DialTimeout         int64              `json:"tdengine.dial_timeout" mapstructure:"tdengine.dial_timeout"`
	MaxIdleConnsPerHost int                `json:"tdengine.max_idle_conns_per_host" mapstructure:"tdengine.max_idle_conns_per_host"`
	Headers             map[string]string  `json:"tdengine.headers" mapstructure:"tdengine.headers"`
	SkipTlsVerify       bool               `json:"tdengine.skip_tls_verify" mapstructure:"tdengine.skip_tls_verify"`

	tlsx.ClientConfig

	header map[string][]string `json:"-"`
	client *http.Client        `json:"-"`
}

type TDengineBasicAuth struct {
	User      string `json:"tdengine.user" mapstructure:"tdengine.user"`
	Password  string `json:"tdengine.password" mapstructure:"tdengine.password"`
	IsEncrypt bool   `json:"tdengine.is_encrypt" mapstructure:"tdengine.is_encrypt"`
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

	for k, v := range tc.Headers {
		kv := strings.Split(v, ":")
		if len(kv) != 2 {
			continue
		}
		tc.header[k] = []string{v}
	}

	if tc.Basic != nil {
		basic := base64.StdEncoding.EncodeToString([]byte(tc.Basic.User + ":" + tc.Basic.Password))
		tc.header["Authorization"] = []string{fmt.Sprintf("Basic %s", basic)}
	}
}

func (tc *Tdengine) QueryTable(query string) (APIResponse, error) {
	var apiResp APIResponse
	req, err := http.NewRequest("POST", tc.Addr+"/rest/sql", strings.NewReader(query))
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
	sql := fmt.Sprintf("show %s", database)
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
	queryMap, ok := query.(map[string]string)
	if !ok {
		return nil, fmt.Errorf("invalid query")
	}
	sql := fmt.Sprintf("select * from %s.%s limit 1", queryMap["database"], queryMap["table"])
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
