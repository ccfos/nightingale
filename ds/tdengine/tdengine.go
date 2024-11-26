package tdengine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	datasource "github.com/ccfos/nightingale/v6/ds"
	"github.com/ccfos/nightingale/v6/ds-kit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"
	"github.com/mitchellh/mapstructure"
	"github.com/toolkits/pkg/logger"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	TDEngineType = "tdengine"
)

// xub todo 正确组织 TDengine 结构
type TDengine struct {
	Addr          string             `json:"tdengine.addr"`
	Timeout       int64              `json:"tdengine.timeout"` // millis
	Basic         *TDengineBasicAuth `json:"tdengine.basic"`
	Headers       map[string]string  `json:"tdengine.headers"`
	SkipTlsVerify bool               `json:"tdengine.skip_tls_verify"`
	ClusterName   string             `json:"tdengine.cluster_name"` // 告警引擎集群名称
	Client        *tdengineClient
	TDOption      TdengineOption
}

type TDengineBasicAuth struct {
	User      string `json:"tdengine.user"`
	Password  string `json:"tdengine.password"`
	IsEncrypt bool   `json:"tdengine.is_encrypt"`
}

type TdengineOption struct {
	DatasourceName string
	Url            string
	BasicAuthUser  string
	BasicAuthPass  string
	Token          string

	Timeout     int64
	DialTimeout int64

	MaxIdleConnsPerHost int

	Headers []string

	tlsx.ClientConfig
}

type tdengineClient struct {
	url    string
	client *http.Client
	header map[string][]string
}

type APIResponse struct {
	Code       int             `json:"code"`
	ColumnMeta [][]interface{} `json:"column_meta"`
	Data       [][]interface{} `json:"data"`
	Rows       int             `json:"rows"`
}

func init() {
	datasource.RegisterDatasource(TDEngineType, new(TDengine))
}

func (td *TDengine) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(TDengine)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (td *TDengine) InitClient() error {

	tc := &tdengineClient{
		url: td.TDOption.Url,
	}
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

	for _, v := range td.TDOption.Headers {
		kv := strings.Split(v, ":")
		if len(kv) != 2 {
			continue
		}
		tc.header[kv[0]] = []string{kv[1]}
	}

	if td.TDOption.BasicAuthUser != "" {
		basic := base64.StdEncoding.EncodeToString([]byte(td.TDOption.BasicAuthUser + ":" + td.TDOption.BasicAuthPass))
		tc.header["Authorization"] = []string{fmt.Sprintf("Basic %s", basic)}
	}

	td.Client = tc
	return nil
}

func (td *TDengine) Equal(other datasource.Datasource) bool {
	otherTD, ok := other.(*TDengine)
	if !ok {
		return false
	}
	if td.TDOption.Url != otherTD.TDOption.Url {
		return false
	}

	if td.TDOption.BasicAuthUser != otherTD.TDOption.BasicAuthUser {
		return false
	}

	if td.TDOption.BasicAuthPass != otherTD.TDOption.BasicAuthPass {
		return false
	}

	if td.TDOption.Token != otherTD.TDOption.Token {
		return false
	}

	if td.TDOption.Timeout != otherTD.TDOption.Timeout {
		return false
	}

	if td.TDOption.DialTimeout != otherTD.TDOption.DialTimeout {
		return false
	}

	if td.TDOption.MaxIdleConnsPerHost != otherTD.TDOption.MaxIdleConnsPerHost {
		return false
	}

	if len(td.TDOption.Headers) != len(otherTD.TDOption.Headers) {
		return false
	}

	for i := 0; i < len(td.TDOption.Headers); i++ {
		if td.TDOption.Headers[i] != otherTD.TDOption.Headers[i] {
			return false
		}
	}
	return true
}

func (td *TDengine) Validate(ctx context.Context) (err error) {
	// xub todo
	return nil
}

func (td *TDengine) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	// xub todo
	return nil, nil
}

func (td *TDengine) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	// xub todo
	return nil, nil
}

func (td *TDengine) QueryData(ctx context.Context, queryParam interface{}) ([]models.DataResp, error) {

	return nil, nil
}

func (td *TDengine) QueryLog(ctx context.Context, queryParam interface{}) ([]interface{}, int64, error) {
	return nil, 0, nil
}

func (td *TDengine) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (td *TDengine) ShowDatabases(context.Context) ([]string, error) {
	var databases []string
	data, err := td.Client.QueryTable("show databases")
	if err != nil {
		return databases, err
	}

	for _, row := range data.Data {
		databases = append(databases, row[0].(string))
	}
	return databases, nil
}

// xub todo move
func (tc *tdengineClient) QueryTable(query string) (APIResponse, error) {
	var apiResp APIResponse
	req, err := http.NewRequest("POST", tc.url+"/rest/sql", strings.NewReader(query))
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

func (td *TDengine) ShowTables(ctx context.Context, database string) ([]string, error) {
	var tables []string
	sql := fmt.Sprintf("show %s.tables", database)
	//if isStable {
	//	sql = fmt.Sprintf("show %s.stables", database)
	//}

	data, err := td.Client.QueryTable(sql)
	if err != nil {
		return tables, err
	}

	for _, row := range data.Data {
		tables = append(tables, row[0].(string))
	}
	return tables, nil
}

func (td *TDengine) DescribeTable(ctx context.Context, query interface{}) ([]*types.ColumnProperty, error) {
	var columns []*types.ColumnProperty
	// xub todo from query
	database, table := "root", "t"
	sql := fmt.Sprintf("select * from %s.%s limit 1", database, table)
	data, err := td.Client.QueryTable(sql)
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
