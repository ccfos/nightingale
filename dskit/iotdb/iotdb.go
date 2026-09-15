package iotdb

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/apache/iotdb-client-go/v2/client"
	"github.com/ccfos/nightingale/v6/dskit/types"
)

type Iotdb struct {
	Addr                string            `json:"iotdb.addr" mapstructure:"iotdb.addr"`
	RPCAddr             string            `json:"iotdb.rpc_addr" mapstructure:"iotdb.rpc_addr"`
	Database            string            `json:"iotdb.database" mapstructure:"iotdb.database"`
	Basic               *IotdbBasicAuth   `json:"iotdb.basic" mapstructure:"iotdb.basic"`
	Timeout             int64             `json:"iotdb.timeout" mapstructure:"iotdb.timeout"`
	DialTimeout         int64             `json:"iotdb.dial_timeout" mapstructure:"iotdb.dial_timeout"`
	MaxIdleConnsPerHost int               `json:"iotdb.max_idle_conns_per_host" mapstructure:"iotdb.max_idle_conns_per_host"`
	Headers             map[string]string `json:"iotdb.headers" mapstructure:"iotdb.headers"`
	SkipTlsVerify       bool              `json:"iotdb.skip_tls_verify" mapstructure:"iotdb.skip_tls_verify"`

	header map[string][]string      `json:"-"`
	client *http.Client             `json:"-"`
	poolMu sync.Mutex               `json:"-"`
	pool   *client.TableSessionPool `json:"-"`

	queryMu sync.RWMutex `json:"-"` // read-lease held by in-flight queries; Close takes the write side
	closed  bool         `json:"-"` // set by Close; guarded by queryMu
}

type IotdbBasicAuth struct {
	User      string `json:"iotdb.user" mapstructure:"iotdb.user"`
	Password  string `json:"iotdb.password" mapstructure:"iotdb.password"`
	IsEncrypt bool   `json:"iotdb.is_encrypt" mapstructure:"iotdb.is_encrypt"`
}

// String deliberately omits authentication material. Datasource instances
// are commonly included in generic initialization/error logs, where the
// default struct formatter would otherwise make it too easy to expose a
// password accidentally.
func (it *Iotdb) String() string {
	if it == nil {
		return "<nil>"
	}
	return fmt.Sprintf("Iotdb{addr:%q rpc_addr:%q database:%q}", it.Addr, it.RPCAddr, it.Database)
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

const (
	defaultRPCPort              = "6667"
	defaultQueryTimeoutMs int64 = 30000
)

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

// Close releases the native RPC session pool and HTTP transport resources.
// It is intentionally optional on datasource.Datasource and is called by dscache
// when a datasource is replaced or removed.
func (it *Iotdb) Close() error {
	// Take the write side of queryMu so we wait for every in-flight query to
	// finish before closing the pool. Closing the pool while another goroutine
	// is inside GetSession would make the client panic on a closed channel.
	it.queryMu.Lock()
	defer it.queryMu.Unlock()
	it.closed = true

	it.poolMu.Lock()
	if it.pool != nil {
		it.pool.Close()
		it.pool = nil
	}
	it.poolMu.Unlock()

	if it.client != nil {
		it.client.CloseIdleConnections()
	}
	return nil
}

var errDatasourceClosed = errors.New("iotdb datasource is closed")

// beginQuery takes the read-side lease that keeps the RPC pool alive for the
// duration of a query. Close takes the write side of the same lock, so it waits
// for in-flight queries and cannot close the pool underneath one — the client's
// SessionPool.GetSession panics if it runs after Close. The returned func
// releases the lease.
func (it *Iotdb) beginQuery() (func(), error) {
	it.queryMu.RLock()
	if it.closed {
		it.queryMu.RUnlock()
		return nil, errDatasourceClosed
	}
	return it.queryMu.RUnlock, nil
}

func (it *Iotdb) rpcEndpoint() (string, string, error) {
	addr := strings.TrimSpace(it.RPCAddr)
	if addr != "" {
		if strings.Contains(addr, "://") {
			u, err := url.Parse(addr)
			if err != nil || u.Hostname() == "" {
				return "", "", fmt.Errorf("invalid iotdb.rpc_addr %q", addr)
			}
			port := u.Port()
			if port == "" {
				port = defaultRPCPort
			}
			return u.Hostname(), port, nil
		}
		if u, err := url.Parse(addr); err == nil && u.Hostname() != "" {
			port := u.Port()
			if port == "" {
				port = defaultRPCPort
			}
			return u.Hostname(), port, nil
		}
		if host, port, err := net.SplitHostPort(addr); err == nil {
			if strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
				return "", "", fmt.Errorf("invalid iotdb.rpc_addr %q", addr)
			}
			return host, port, nil
		}
		if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
			host := strings.Trim(addr, "[]")
			if host != "" {
				return host, defaultRPCPort, nil
			}
			return "", "", fmt.Errorf("invalid iotdb.rpc_addr %q", addr)
		}
		if strings.Count(addr, ":") == 1 {
			parts := strings.SplitN(addr, ":", 2)
			if strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
				return parts[0], parts[1], nil
			}
			return "", "", fmt.Errorf("invalid iotdb.rpc_addr %q", addr)
		}
		if !strings.Contains(addr, ":") && !strings.ContainsAny(addr, "/?#") {
			return addr, defaultRPCPort, nil
		}
		if strings.Count(addr, ":") > 1 {
			host := strings.Trim(addr, "[]")
			if net.ParseIP(host) != nil {
				return host, defaultRPCPort, nil
			}
		}
		return "", "", fmt.Errorf("invalid iotdb.rpc_addr %q", addr)
	}
	raw := strings.TrimSpace(it.Addr)
	if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
		return u.Hostname(), defaultRPCPort, nil
	}
	if !strings.Contains(raw, "://") {
		if u, err := url.Parse("http://" + raw); err == nil && u.Hostname() != "" {
			return u.Hostname(), defaultRPCPort, nil
		}
	}
	return "", "", fmt.Errorf("cannot derive IoTDB RPC host; set iotdb.rpc_addr")
}

// tablePool lazily creates the single shared native-client session pool. The
// pool carries no database in its configuration: every query selects its own
// database with a USE statement on the session it checks out, so a session's
// leftover database state can never leak into the next query.
func (it *Iotdb) tablePool() (*client.TableSessionPool, error) {
	it.poolMu.Lock()
	defer it.poolMu.Unlock()
	if it.pool != nil {
		return it.pool, nil
	}
	host, port, err := it.rpcEndpoint()
	if err != nil {
		return nil, err
	}
	connectTimeout := int(it.DialTimeout)
	if connectTimeout <= 0 {
		connectTimeout = 10000
	}
	waitTimeout := connectTimeout
	if waitTimeout < 1000 {
		waitTimeout = 1000
	}
	maxSize := it.MaxIdleConnsPerHost
	if maxSize <= 0 || maxSize > 64 {
		maxSize = 8
	}
	user, password := "", ""
	if it.Basic != nil {
		user, password = it.Basic.User, it.Basic.Password
	}
	conf := &client.PoolConfig{Host: host, Port: port, UserName: user, Password: password}
	pool := client.NewTableSessionPool(conf, maxSize, connectTimeout, waitTimeout, false)
	it.pool = &pool
	return it.pool, nil
}

func (it *Iotdb) rpcTarget() string {
	host, port, err := it.rpcEndpoint()
	if err != nil {
		return "<unknown>"
	}
	return net.JoinHostPort(host, port)
}

// TestRPC performs a real table-model RPC round-trip for Save & Test and
// deliberately does not log credentials or SQL. When a default database is
// configured it is validated with a USE statement; otherwise the connection is
// checked with a database-independent query.
func (it *Iotdb) TestRPC(ctx context.Context) error {
	release, err := it.beginQuery()
	if err != nil {
		return err
	}
	defer release()

	pool, err := it.tablePool()
	if err != nil {
		return err
	}
	session, err := pool.GetSession()
	if err != nil {
		return fmt.Errorf("cannot connect to IoTDB RPC at %s: %w", it.rpcTarget(), err)
	}
	defer session.Close()
	if database := strings.TrimSpace(it.Database); database != "" {
		// Validating the default database with USE exercises the exact
		// session-database path queries rely on. ExecuteNonQueryStatement has
		// no timeout parameter and does not observe ctx cancellation, so a
		// hung server can stall this one-shot diagnostic; that is accepted.
		if err := session.ExecuteNonQueryStatement("USE " + quoteIdentifier(database)); err != nil {
			return fmt.Errorf("cannot USE database %q: %w", database, err)
		}
		return nil
	}
	timeout := it.queryTimeoutMs(ctx)
	result, err := session.ExecuteQueryStatement("show databases", &timeout)
	if err != nil {
		return fmt.Errorf("IoTDB RPC query failed: %w", err)
	}
	return result.Close()
}

func (it *Iotdb) queryTimeoutMs(ctx context.Context) int64 {
	timeout := time.Duration(it.Timeout) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(defaultQueryTimeoutMs) * time.Millisecond
	}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	ms := timeout.Milliseconds()
	if ms < 1 {
		ms = 1
	}
	return ms
}

// QueryTable executes table-model SQL over native RPC. The requested database
// is selected on the checked-out session with a USE statement before the query
// runs, so a session's leftover database state from an earlier query can never
// change which database the statement resolves against.
func (it *Iotdb) QueryTable(ctx context.Context, database, query string, rowLimit int) (APIResponse, error) {
	var response APIResponse
	if err := ctx.Err(); err != nil {
		return response, err
	}
	release, err := it.beginQuery()
	if err != nil {
		return response, err
	}
	defer release()

	database = strings.TrimSpace(database)
	if database == "" {
		database = strings.TrimSpace(it.Database)
	}
	if database == "" {
		return response, fmt.Errorf("IoTDB database is required: set a datasource default database or query database")
	}
	pool, err := it.tablePool()
	if err != nil {
		return response, err
	}
	session, err := pool.GetSession()
	if err != nil {
		return response, fmt.Errorf("cannot connect to IoTDB RPC at %s: %w", it.rpcTarget(), err)
	}
	defer session.Close()
	result, err := queryOnSession(session, database, query, it.queryTimeoutMs(ctx))
	if err != nil {
		return response, err
	}
	defer result.Close()
	columns := result.GetColumnNames()
	response.ColumnNames = append([]string(nil), columns...)
	response.Expressions = append([]string(nil), columns...)
	response.Values = make([][]interface{}, 0)
	for {
		hasNext, err := result.Next()
		if err != nil {
			return response, err
		}
		if !hasNext {
			break
		}
		row := make([]interface{}, len(columns))
		for i := range columns {
			value, err := result.GetObjectByIndex(int32(i + 1))
			if err != nil {
				return response, err
			}
			row[i] = value
		}
		response.Values = append(response.Values, row)
		if rowLimit > 0 && len(response.Values) >= rowLimit {
			break
		}
	}
	return response, nil
}

// queryOnSession selects the database on the session and runs the query. It is
// split out so the USE-before-query ordering and error propagation can be
// unit-tested against a fake ITableSession without a live server.
func queryOnSession(session client.ITableSession, database, query string, timeoutMs int64) (*client.SessionDataSet, error) {
	if err := session.ExecuteNonQueryStatement("USE " + quoteIdentifier(database)); err != nil {
		return nil, fmt.Errorf("cannot USE database %q: %w", database, err)
	}
	result, err := session.ExecuteQueryStatement(strings.TrimSpace(query), ptrInt64(timeoutMs))
	if err != nil {
		return nil, fmt.Errorf("IoTDB RPC query failed: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("IoTDB RPC query returned no result set")
	}
	return result, nil
}

func ptrInt64(value int64) *int64 { return &value }

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func (it *Iotdb) queryREST(ctx context.Context, database, query string, rowLimit int) (APIResponse, error) {
	var apiResp APIResponse
	if strings.TrimSpace(it.Addr) == "" {
		return apiResp, fmt.Errorf("IoTDB REST address is required for metadata queries")
	}
	if it.client == nil {
		it.InitCli()
	}

	body, err := json.Marshal(queryRequest{
		Database: database,
		SQL:      query,
		RowLimit: rowLimit,
	})
	if err != nil {
		return apiResp, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(it.Addr, "/")+"/rest/table/v1/query", bytes.NewReader(body))
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
		// IoTDB 在鉴权失败/SQL 错误等情况下会把可读原因放在 body，读有限字节拼进错误便于定位。
		body, _ := io.ReadAll(io.LimitReader(limitedReader, 1024))
		if msg := strings.TrimSpace(string(body)); msg != "" {
			return apiResp, fmt.Errorf("HTTP error, status: %s, body: %s", resp.Status, msg)
		}
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

func (it *Iotdb) ShowDatabases(ctx context.Context) ([]string, error) {
	resp, err := it.queryREST(ctx, "", "show databases", 0)
	if err != nil {
		return nil, err
	}
	return filterDatabases(firstColumn(resp)), nil
}

func (it *Iotdb) ShowTables(ctx context.Context, database string) ([]string, error) {
	sql := "show tables"
	if database == "" {
		resp, err := it.queryREST(ctx, "", sql, 0)
		if err != nil {
			return nil, err
		}
		return firstColumn(resp), nil
	}

	resp, err := it.queryREST(ctx, database, sql, 0)
	if err != nil {
		return nil, err
	}
	return firstColumn(resp), nil
}

func (it *Iotdb) DescribeTable(ctx context.Context, query interface{}) ([]*types.ColumnProperty, error) {
	queryMap, ok := query.(map[string]string)
	if !ok {
		return nil, fmt.Errorf("invalid query")
	}

	database := queryMap["database"]
	table := queryMap["table"]
	if table == "" {
		return nil, fmt.Errorf("table is empty")
	}

	resp, err := it.queryREST(ctx, database, fmt.Sprintf("describe %s", table), 0)
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
