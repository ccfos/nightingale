package tdengine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
)

type TdengineQuery struct {
	From     string `json:"from"`
	Interval int64  `json:"interval"`
	Keys     Keys   `json:"keys"`
	Query    string `json:"query"` // 查询条件
	Ref      string `json:"ref"`   // 变量
	To       string `json:"to"`
}

type Keys struct {
	LabelKey   string `json:"labelKey"`  // 多个用空格分隔
	MetricKey  string `json:"metricKey"` // 多个用空格分隔
	TimeFormat string `json:"timeFormat"`
}

type TdengineClientMap struct {
	sync.RWMutex
	ctx           *ctx.Context
	heartbeat     aconf.HeartbeatConfig
	ReaderClients map[int64]*tdengineClient
}

func (pc *TdengineClientMap) Set(datasourceId int64, r *tdengineClient) {
	if r == nil {
		return
	}
	pc.Lock()
	defer pc.Unlock()
	pc.ReaderClients[datasourceId] = r
}

func (pc *TdengineClientMap) GetDatasourceIds() []int64 {
	pc.RLock()
	defer pc.RUnlock()
	var datasourceIds []int64
	for k := range pc.ReaderClients {
		datasourceIds = append(datasourceIds, k)
	}

	return datasourceIds
}

func (pc *TdengineClientMap) GetCli(datasourceId int64) *tdengineClient {
	pc.RLock()
	defer pc.RUnlock()
	c := pc.ReaderClients[datasourceId]
	return c
}

func (pc *TdengineClientMap) IsNil(datasourceId int64) bool {
	pc.RLock()
	defer pc.RUnlock()

	c, exists := pc.ReaderClients[datasourceId]
	if !exists {
		return true
	}

	return c == nil
}

// Hit 根据当前有效的 datasourceId 和规则的 datasourceId 配置计算有效的cluster列表
func (pc *TdengineClientMap) Hit(datasourceIds []int64) []int64 {
	pc.RLock()
	defer pc.RUnlock()
	dsIds := make([]int64, 0, len(pc.ReaderClients))
	if len(datasourceIds) == 1 && datasourceIds[0] == models.DatasourceIdAll {
		for c := range pc.ReaderClients {
			dsIds = append(dsIds, c)
		}
		return dsIds
	}

	for dsId := range pc.ReaderClients {
		for _, id := range datasourceIds {
			if id == dsId {
				dsIds = append(dsIds, id)
				continue
			}
		}
	}
	return dsIds
}

func (pc *TdengineClientMap) Reset() {
	pc.Lock()
	defer pc.Unlock()

	pc.ReaderClients = make(map[int64]*tdengineClient)
}

func (pc *TdengineClientMap) Del(datasourceId int64) {
	pc.Lock()
	defer pc.Unlock()
	delete(pc.ReaderClients, datasourceId)
}

type tdengineClient struct {
	url    string
	client *http.Client
	header map[string][]string
}

func newTdengine(po TdengineOption) *tdengineClient {
	tc := &tdengineClient{
		url: po.Url,
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

	for _, v := range po.Headers {
		kv := strings.Split(v, ":")
		if len(kv) != 2 {
			continue
		}
		tc.header[kv[0]] = []string{kv[1]}
	}

	if po.BasicAuthUser != "" {
		basic := base64.StdEncoding.EncodeToString([]byte(po.BasicAuthUser + ":" + po.BasicAuthPass))
		tc.header["Authorization"] = []string{fmt.Sprintf("Basic %s", basic)}
	}

	return tc
}

type APIResponse struct {
	Code       int             `json:"code"`
	ColumnMeta [][]interface{} `json:"column_meta"`
	Data       [][]interface{} `json:"data"`
	Rows       int             `json:"rows"`
}

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

	if resp.StatusCode != http.StatusOK {
		return apiResp, fmt.Errorf("HTTP error, status: %s", resp.Status)
	}
	err = json.NewDecoder(resp.Body).Decode(&apiResp)
	return apiResp, err
}

func (tc *tdengineClient) QueryLog(query interface{}) (APIResponse, error) {
	b, err := json.Marshal(query)
	if err != nil {
		return APIResponse{}, err
	}
	var q TdengineQuery
	err = json.Unmarshal(b, &q)
	if err != nil {
		return APIResponse{}, err
	}

	if q.Interval == 0 {
		q.Interval = 60
	}

	if q.From == "" {
		// 2023-09-21T05:37:30.000Z format
		to := time.Now().Unix()
		q.To = time.Unix(to, 0).UTC().Format(time.RFC3339)
		from := to - q.Interval
		q.From = time.Unix(from, 0).UTC().Format(time.RFC3339)
	}

	replacements := map[string]string{
		"$from":     fmt.Sprintf("'%s'", q.From),
		"$to":       fmt.Sprintf("'%s'", q.To),
		"$interval": fmt.Sprintf("%ds", q.Interval),
	}

	for key, val := range replacements {
		q.Query = strings.ReplaceAll(q.Query, key, val)
	}

	if !strings.Contains(q.Query, "limit") {
		q.Query = q.Query + " limit 200"
	}

	data, err := tc.QueryTable(q.Query)
	if err != nil {
		return data, err
	}

	return TimeFormat(data, q.Keys.TimeFormat), err
}

func TimeFormat(src APIResponse, timeFormat string) APIResponse {
	if timeFormat == "" {
		return src
	}

	tsIdx := -1
	for colIndex, colData := range src.ColumnMeta {
		//  类型参考 https://docs.taosdata.com/taos-sql/data-type/
		colType, ok := colData[1].(string)
		if !ok {
			logger.Warningf("unexpected column type: %v", colData[1])
			return src
		}

		if colType == "TIMESTAMP" {
			tsIdx = colIndex
			break
		}
	}

	if tsIdx == -1 {
		return src
	}

	for i := range src.Data {
		ts, ok := src.Data[i][tsIdx].(string)
		if !ok {
			logger.Warningf("unexpected timestamp type: %v", src.Data[i][tsIdx])
			continue
		}

		t, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			logger.Warningf("parse %v timestamp failed: %v", src.Data[i], err)
			continue
		}

		src.Data[i][tsIdx] = t.In(time.Local).Format(timeFormat)
	}
	return src
}

func (tc *tdengineClient) Query(query interface{}) ([]*models.DataResp, error) {
	b, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	var q TdengineQuery
	err = json.Unmarshal(b, &q)
	if err != nil {
		return nil, err
	}

	if q.Interval == 0 {
		q.Interval = 60
	}

	if q.From == "" {
		// 2023-09-21T05:37:30.000Z format
		to := time.Now().Unix()
		q.To = time.Unix(to, 0).UTC().Format(time.RFC3339)
		from := to - q.Interval
		q.From = time.Unix(from, 0).UTC().Format(time.RFC3339)
	}

	replacements := map[string]string{
		"$from":     fmt.Sprintf("'%s'", q.From),
		"$to":       fmt.Sprintf("'%s'", q.To),
		"$interval": fmt.Sprintf("%ds", q.Interval),
	}

	for key, val := range replacements {
		q.Query = strings.ReplaceAll(q.Query, key, val)
	}

	data, err := tc.QueryTable(q.Query)
	if err != nil {
		return nil, err
	}
	logger.Debugf("tdengine query:%s result: %+v", q.Query, data)

	return ConvertToTStData(data, q.Keys, q.Ref)
}

// get tdendgine databases
func (tc *tdengineClient) GetDatabases() ([]string, error) {
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

// get tdendgine tables by database
func (tc *tdengineClient) GetTables(database string, isStable bool) ([]string, error) {
	var tables []string
	sql := fmt.Sprintf("show %s.tables", database)
	if isStable {
		sql = fmt.Sprintf("show %s.stables", database)
	}

	data, err := tc.QueryTable(sql)
	if err != nil {
		return tables, err
	}

	for _, row := range data.Data {
		tables = append(tables, row[0].(string))
	}
	return tables, nil
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int    `json:"size"`
}

func (tc *tdengineClient) GetColumns(database, table string) ([]Column, error) {
	var columns []Column
	sql := fmt.Sprintf("select * from %s.%s limit 1", database, table)
	data, err := tc.QueryTable(sql)
	if err != nil {
		return columns, err
	}
	for _, row := range data.ColumnMeta {
		column := Column{
			Name: row[0].(string),
			Type: row[1].(string),
			Size: int(row[2].(float64)),
		}
		columns = append(columns, column)
	}

	return columns, nil
}

// {
// 	"code": 0,
// 	"column_meta": [
// 	  ["ts", "TIMESTAMP", 8],
// 	  ["count", "BIGINT", 8],
// 	  ["endpoint", "VARCHAR", 45],
// 	  ["status_code", "INT", 4],
// 	  ["client_ip", "VARCHAR", 40],
// 	  ["request_method", "VARCHAR", 15],
// 	  ["request_uri", "VARCHAR", 128]
// 	],
// 	"data": [
// 	  [
// 		"2022-06-29T05:50:55.401Z",
// 		2,
// 		"LAPTOP-NNKFTLTG:6041",
// 		200,
// 		"172.23.208.1",
// 		"POST",
// 		"/rest/sql"
// 	  ],
// 	  [
// 		"2022-06-29T05:52:16.603Z",
// 		1,
// 		"LAPTOP-NNKFTLTG:6041",
// 		200,
// 		"172.23.208.1",
// 		"POST",
// 		"/rest/sql"
// 	  ]
// 	],
// 	"rows": 2
//   }

// {
//     "dat": [
//         {
//             "ref": "",
//             "metric": {
//                 "__name__": "count",
//                 "host":"host1"
//             },
//             "values": [
//                 [
//                     1693219500,
//                     12
//                 ]
//             ]
//         }
//     ],
//     "err": ""
// }

func ConvertToTStData(src APIResponse, key Keys, ref string) ([]*models.DataResp, error) {
	metricIdxMap := make(map[string]int)
	labelIdxMap := make(map[string]int)

	metricMap := make(map[string]struct{})
	if key.MetricKey != "" {
		metricList := strings.Split(key.MetricKey, " ")
		for _, metric := range metricList {
			metricMap[metric] = struct{}{}
		}
	}

	labelMap := make(map[string]string)
	if key.LabelKey != "" {
		labelList := strings.Split(key.LabelKey, " ")
		for _, label := range labelList {
			labelMap[label] = label
		}
	}

	tsIdx := -1
	for colIndex, colData := range src.ColumnMeta {
		//  类型参考 https://docs.taosdata.com/taos-sql/data-type/
		colName := colData[0].(string)
		colType := colData[1].(string)
		switch colType {
		case "TIMESTAMP":
			tsIdx = colIndex
		case "BIGINT", "INT", "INT UNSIGNED", "BIGINT UNSIGNED", "FLOAT", "DOUBLE",
			"SMALLINT", "SMALLINT UNSIGNED", "TINYINT", "TINYINT UNSIGNED", "BOOL":
			if len(metricMap) > 0 {
				if _, ok := metricMap[colName]; !ok {
					continue
				}
				metricIdxMap[colName] = colIndex
			} else {
				metricIdxMap[colName] = colIndex
			}

		default:
			if len(labelMap) > 0 {
				if _, ok := labelMap[colName]; !ok {
					continue
				}
				labelIdxMap[colName] = colIndex
			} else {
				labelIdxMap[colName] = colIndex
			}
		}
	}

	if tsIdx == -1 {
		return nil, fmt.Errorf("timestamp column not found, please check your query")
	}

	var result []*models.DataResp
	m := make(map[string]*models.DataResp)
	for _, row := range src.Data {
		for metricName, metricIdx := range metricIdxMap {
			value, err := interfaceToFloat64(row[metricIdx])
			if err != nil {
				logger.Warningf("parse %v value failed: %v", row, err)
				continue
			}

			metric := make(model.Metric)
			for labelName, labelIdx := range labelIdxMap {
				metric[model.LabelName(labelName)] = model.LabelValue(row[labelIdx].(string))
			}

			metric[model.MetricNameLabel] = model.LabelValue(metricName)

			// transfer 2022-06-29T05:52:16.603Z to unix timestamp
			t, err := time.Parse(time.RFC3339, row[tsIdx].(string))
			if err != nil {
				logger.Warningf("parse %v timestamp failed: %v", row, err)
				continue
			}

			timestamp := t.Unix()
			if _, ok := m[metric.String()]; !ok {
				m[metric.String()] = &models.DataResp{
					Metric: metric,
					Values: [][]float64{
						{float64(timestamp), value},
					},
				}
			} else {
				m[metric.String()].Values = append(m[metric.String()].Values, []float64{float64(timestamp), value})
			}
		}
	}

	for _, v := range m {
		v.Ref = ref
		result = append(result, v)
	}

	return result, nil
}

func interfaceToFloat64(input interface{}) (float64, error) {
	// Check for the kind of the value first
	if input == nil {
		return 0, fmt.Errorf("unsupported type: %T", input)
	}

	kind := reflect.TypeOf(input).Kind()
	switch kind {
	case reflect.Float64:
		return input.(float64), nil
	case reflect.Float32:
		return float64(input.(float32)), nil
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Int8, reflect.Int16:
		return float64(reflect.ValueOf(input).Int()), nil
	case reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Uint8, reflect.Uint16:
		return float64(reflect.ValueOf(input).Uint()), nil
	case reflect.String:
		return strconv.ParseFloat(input.(string), 64)
	case reflect.Bool:
		if input.(bool) {
			return 1.0, nil
		}
		return 0.0, nil
	default:
		return 0, fmt.Errorf("unsupported type: %T", input)
	}
}
