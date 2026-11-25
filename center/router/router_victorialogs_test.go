package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	dsvlogs "github.com/ccfos/nightingale/v6/datasource/victorialogs"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/pkg/aop"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// vlogsFieldsUpstream is a minimal VictoriaLogs stub for the field suggestion APIs.
type vlogsFieldsUpstream struct {
	mu             sync.Mutex
	requests       map[string][]url.Values
	fieldNamesJSON string
}

func newVlogsFieldsUpstream(t *testing.T) (*vlogsFieldsUpstream, *httptest.Server) {
	t.Helper()
	up := &vlogsFieldsUpstream{requests: make(map[string][]url.Values)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		up.mu.Lock()
		up.requests[r.URL.Path] = append(up.requests[r.URL.Path], r.Form)
		up.mu.Unlock()

		switch r.URL.Path {
		case "/select/logsql/field_names":
			if up.fieldNamesJSON != "" {
				fmt.Fprintln(w, up.fieldNamesJSON)
				return
			}
			fmt.Fprintln(w, `{"values":[{"value":"status","hits":1}]}`)
		case "/select/logsql/field_values":
			fmt.Fprintln(w, `{"values":[{"value":"500","hits":1}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return up, server
}

func (up *vlogsFieldsUpstream) requestCount(path string) int {
	up.mu.Lock()
	defer up.mu.Unlock()
	return len(up.requests[path])
}

func (up *vlogsFieldsUpstream) lastRequest(path string) url.Values {
	up.mu.Lock()
	defer up.mu.Unlock()
	list := up.requests[path]
	if len(list) == 0 {
		return nil
	}
	return list[len(list)-1]
}

// setupVLogsFieldsRouter registers datasource 101 and returns a router that runs the
// real recovery middleware. AnonymousAccess.PromQuerier skips CheckDsPerm, which is
// out of scope here.
func setupVLogsFieldsRouter(t *testing.T, serverURL string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ds := &dsvlogs.VictoriaLogs{}
	ds.VictorialogsAddr = serverURL
	ds.MaxQueryRows = 1000
	dscache.DsCache.Put(dsvlogs.VictoriaLogsType, 101, ds)
	t.Cleanup(func() { dscache.DsCache.Delete(dsvlogs.VictoriaLogsType, 101) })

	rt := &Router{}
	rt.Center.AnonymousAccess.PromQuerier = true

	r := gin.New()
	r.Use(aop.Recovery())
	r.POST("/api/n9e/victorialogs-field-names", rt.QueryVictoriaLogsFieldNames)
	r.POST("/api/n9e/victorialogs-field-values", rt.QueryVictoriaLogsFieldValues)
	return r
}

func postVLogsJSON(t *testing.T, r *gin.Engine, path, body string) (*httptest.ResponseRecorder, map[string]json.RawMessage) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	var raw map[string]json.RawMessage
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	}
	return w, raw
}

// _msg is the log message body: it is selectable as a filter field, while the
// metadata fields and _stream.* stay out of the suggestions. The list is truncated
// after filtering and sorting so it matches the requested limit.
func TestVictoriaLogsFieldNamesKeepsMsgAndTruncates(t *testing.T) {
	up, server := newVlogsFieldsUpstream(t)
	up.fieldNamesJSON = `{"values":[
		{"value":"zeta","hits":1},
		{"value":"_msg","hits":9},
		{"value":"alpha","hits":2},
		{"value":"_stream.pod","hits":8},
		{"value":"beta","hits":3},
		{"value":"gamma","hits":4}
	]}`
	r := setupVLogsFieldsRouter(t, server.URL)

	w, raw := postVLogsJSON(t, r, "/api/n9e/victorialogs-field-names",
		`{"cate":"victorialogs","datasource_id":101,"scope":"field","query":"*","start":1763600000,"end":1763603600,"limit":3}`)
	require.Equal(t, http.StatusOK, w.Code)

	var fields []struct {
		Field string `json:"field"`
	}
	require.NoError(t, json.Unmarshal(raw["dat"], &fields))
	require.Len(t, fields, 3)
	require.Equal(t, []string{"_msg", "alpha", "beta"}, []string{fields[0].Field, fields[1].Field, fields[2].Field})
	require.Equal(t, "3", up.lastRequest("/select/logsql/field_names").Get("limit"))
}

// _msg values stay empty (a top-N of message bodies is not meaningful), and the
// upstream field_values endpoint must not be called for it.
func TestVictoriaLogsFieldValuesKeepsMsgBlocked(t *testing.T) {
	up, server := newVlogsFieldsUpstream(t)
	r := setupVLogsFieldsRouter(t, server.URL)

	w, raw := postVLogsJSON(t, r, "/api/n9e/victorialogs-field-values",
		`{"cate":"victorialogs","datasource_id":101,"scope":"field","query":"*","start":1763600000,"end":1763603600,"field":"_msg"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 0, up.requestCount("/select/logsql/field_values"))
	require.JSONEq(t, `[]`, string(raw["dat"]))
}
