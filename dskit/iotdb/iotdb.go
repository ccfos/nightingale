package iotdb

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/types"
)

type Iotdb struct {
	Addr                string            `json:"iotdb.addr" mapstructure:"iotdb.addr"`
	Basic               *IotdbBasicAuth   `json:"iotdb.basic" mapstructure:"iotdb.basic"`
	Timeout             int64             `json:"iotdb.timeout" mapstructure:"iotdb.timeout"`
	DialTimeout         int64             `json:"iotdb.dial_timeout" mapstructure:"iotdb.dial_timeout"`
	MaxIdleConnsPerHost int               `json:"iotdb.max_idle_conns_per_host" mapstructure:"iotdb.max_idle_conns_per_host"`
	Headers             map[string]string `json:"iotdb.headers" mapstructure:"iotdb.headers"`
	SkipTlsVerify       bool              `json:"iotdb.skip_tls_verify" mapstructure:"iotdb.skip_tls_verify"`

	header map[string][]string `json:"-"`
	client *http.Client        `json:"-"`
}

type IotdbBasicAuth struct {
	User      string `json:"iotdb.user" mapstructure:"iotdb.user"`
	Password  string `json:"iotdb.password" mapstructure:"iotdb.password"`
	IsEncrypt bool   `json:"iotdb.is_encrypt" mapstructure:"iotdb.is_encrypt"`
}

type APIResponse struct {
	Code        int             `json:"code"`
	Message     string          `json:"message"`
	Expressions []string        `json:"expressions"`
	ColumnNames []string        `json:"column_names"`
	Timestamps  []int64         `json:"timestamps"`
	Values      [][]interface{} `json:"values"`
}

type queryRequest struct {
	Database string `json:"database,omitempty"`
	SQL      string `json:"sql"`
	RowLimit int    `json:"row_limit,omitempty"`
}

func (it *Iotdb) InitCli() {
	timeout := time.Duration(it.Timeout) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	dialTimeout := time.Duration(it.DialTimeout) * time.Millisecond
	if dialTimeout <= 0 {
		dialTimeout = 30 * time.Second
	}

	maxIdleConnsPerHost := it.MaxIdleConnsPerHost
	if maxIdleConnsPerHost <= 0 {
		maxIdleConnsPerHost = 100
	}

	it.client = &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: it.SkipTlsVerify},
			DialContext: (&net.Dialer{
				Timeout:   dialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConnsPerHost:   maxIdleConnsPerHost,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableCompression:    true,
		},
	}

	it.header = map[string][]string{
		"Connection":   {"keep-alive"},
		"Content-Type": {"application/json"},
	}

	for k, v := range it.Headers {
		it.header[k] = []string{v}
	}

	if it.Basic != nil {
		basic := base64.StdEncoding.EncodeToString([]byte(it.Basic.User + ":" + it.Basic.Password))
		it.header["Authorization"] = []string{fmt.Sprintf("Basic %s", basic)}
	}
}

func (it *Iotdb) QueryTable(database, query string, rowLimit int) (APIResponse, error) {
	var apiResp APIResponse

	body, err := json.Marshal(queryRequest{
		Database: database,
		SQL:      query,
		RowLimit: rowLimit,
	})
	if err != nil {
		return apiResp, err
	}

	req, err := http.NewRequest("POST", strings.TrimRight(it.Addr, "/")+"/rest/table/v1/query", bytes.NewReader(body))
	if err != nil {
		return apiResp, err
	}

	for k, v := range it.header {
		req.Header[k] = v
	}

	resp, err := it.client.Do(req)
	if err != nil {
		return apiResp, err
	}
	defer resp.Body.Close()

	maxSize := int64(10 * 1024 * 1024)
	limitedReader := http.MaxBytesReader(nil, resp.Body, maxSize)

	if resp.StatusCode != http.StatusOK {
		return apiResp, fmt.Errorf("HTTP error, status: %s", resp.Status)
	}

	if err := json.NewDecoder(limitedReader).Decode(&apiResp); err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			return apiResp, fmt.Errorf("response body exceeds 10MB limit")
		}
		return apiResp, err
	}

	if apiResp.Code != 0 && apiResp.Code != http.StatusOK {
		if apiResp.Message != "" {
			return apiResp, fmt.Errorf("iotdb query failed, code: %d, message: %s", apiResp.Code, apiResp.Message)
		}
		return apiResp, fmt.Errorf("iotdb query failed, code: %d", apiResp.Code)
	}

	return apiResp, nil
}

func (it *Iotdb) ShowDatabases(_ context.Context) ([]string, error) {
	resp, err := it.QueryTable("", "show databases", 0)
	if err != nil {
		return nil, err
	}
	return filterDatabases(firstColumn(resp)), nil
}

func (it *Iotdb) ShowTables(_ context.Context, database string) ([]string, error) {
	sql := "show tables"
	if database == "" {
		resp, err := it.QueryTable("", sql, 0)
		if err != nil {
			return nil, err
		}
		return firstColumn(resp), nil
	}

	resp, err := it.QueryTable(database, sql, 0)
	if err != nil {
		return nil, err
	}
	return firstColumn(resp), nil
}

func (it *Iotdb) DescribeTable(_ context.Context, query interface{}) ([]*types.ColumnProperty, error) {
	queryMap, ok := query.(map[string]string)
	if !ok {
		return nil, fmt.Errorf("invalid query")
	}

	database := queryMap["database"]
	table := queryMap["table"]
	if table == "" {
		return nil, fmt.Errorf("table is empty")
	}

	resp, err := it.QueryTable(database, fmt.Sprintf("describe %s", table), 0)
	if err != nil {
		return nil, err
	}

	rows := rowsFromResponse(resp)
	columns := make([]*types.ColumnProperty, 0, len(rows))
	for _, row := range rows {
		field := firstNonEmptyString(row, "column_name", "ColumnName", "column", "Field")
		colType := firstNonEmptyString(row, "data_type", "DataType", "type", "Type")
		if field == "" || colType == "" {
			continue
		}

		columns = append(columns, &types.ColumnProperty{
			Field: field,
			Type:  colType,
		})
	}

	return columns, nil
}

func firstColumn(resp APIResponse) []string {
	rows := rowsFromResponse(resp)
	if len(rows) == 0 {
		return []string{}
	}

	column := ""
	if len(resp.ColumnNames) > 0 {
		column = resp.ColumnNames[0]
	} else if len(resp.Expressions) > 0 {
		column = resp.Expressions[0]
	}
	if column == "" {
		return []string{}
	}

	result := make([]string, 0, len(rows))
	for _, row := range rows {
		item, exists := row[column]
		if !exists || item == nil {
			continue
		}
		result = append(result, fmt.Sprintf("%v", item))
	}
	return result
}

func filterDatabases(databases []string) []string {
	systemDatabases := map[string]struct{}{
		"information_schema": {},
	}

	filtered := make([]string, 0, len(databases))
	for _, database := range databases {
		name := strings.TrimSpace(database)
		if name == "" {
			continue
		}
		if _, isSystem := systemDatabases[strings.ToLower(name)]; isSystem {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

func rowsFromResponse(resp APIResponse) []map[string]interface{} {
	columns := resp.ColumnNames
	if len(columns) == 0 {
		columns = resp.Expressions
	}

	if len(columns) == 0 || len(resp.Values) == 0 {
		return []map[string]interface{}{}
	}

	// IoTDB table model commonly returns row-oriented values:
	// values = [[col1, col2], [col1, col2], ...]
	if len(resp.Values[0]) == len(columns) {
		rows := make([]map[string]interface{}, 0, len(resp.Values))
		for _, rawRow := range resp.Values {
			row := make(map[string]interface{}, len(columns))
			for colIdx, colName := range columns {
				if colIdx >= len(rawRow) {
					row[colName] = nil
					continue
				}
				row[colName] = rawRow[colIdx]
			}
			rows = append(rows, row)
		}
		return rows
	}

	rowCount := 0
	for _, col := range resp.Values {
		if len(col) > rowCount {
			rowCount = len(col)
		}
	}

	rows := make([]map[string]interface{}, 0, rowCount)
	for rowIdx := 0; rowIdx < rowCount; rowIdx++ {
		row := make(map[string]interface{}, len(columns))
		for colIdx, colName := range columns {
			if colIdx >= len(resp.Values) || rowIdx >= len(resp.Values[colIdx]) {
				row[colName] = nil
				continue
			}
			row[colName] = resp.Values[colIdx][rowIdx]
		}
		rows = append(rows, row)
	}

	return rows
}

func firstNonEmptyString(row map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := row[key]; ok && val != nil {
			str := strings.TrimSpace(fmt.Sprintf("%v", val))
			if str != "" && str != "<nil>" {
				return str
			}
		}
	}
	return ""
}
