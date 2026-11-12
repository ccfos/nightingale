package loki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryRange_SendsParametersAndAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/loki/api/v1/query_range", r.URL.Path)
		assert.Equal(t, url.Values{
			"query": {`{app="api"} |= "error"`}, "start": {"1700000000000000000"},
			"end": {"1700000060000000000"}, "step": {"10s"}, "limit": {"25"}, "direction": {"backward"},
		}, r.URL.Query())
		assert.Equal(t, "team", r.Header.Get("X-Scope-OrgID"))
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "reader", user)
		assert.Equal(t, "secret", pass)
		_, _ = w.Write([]byte(`{
  "status": "success",
  "data": {
    "resultType": "matrix",
    "result": [
      {
        "metric": {
          "app": "api"
        },
        "values": [
          [
            1700000000,
            "2"
          ]
        ]
      }
    ]
  }
}`))
	}))
	defer server.Close()
	client := &Loki{
		LokiAddr: server.URL + "/loki", LokiBasic: LokiBasicAuth{LokiUser: "reader", LokiPass: "secret"},
		Headers: map[string]string{"X-Scope-OrgID": "team"},
	}

	got, err := client.QueryRange(context.Background(), `{app="api"} |= "error"`, 1700000000, 1700000060, "10s", 25, "backward")

	require.NoError(t, err)
	require.Equal(t, "matrix", got.Data.ResultType)
	require.Equal(t, "api", got.Data.Result[0].Metric["app"])
	require.Equal(t, [][]interface{}{{float64(1700000000), "2"}}, got.Data.Result[0].Values)
}

func TestQueryInstant_SendsTimestampAndParsesVector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/query", r.URL.Path)
		assert.Equal(t, "1700000000123000000", r.URL.Query().Get("time"))
		assert.Equal(t, "sum(rate({app=\"api\"}[1m]))", r.URL.Query().Get("query"))
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1700000000,"3"]}]}}`))
	}))
	defer server.Close()
	client := &Loki{LokiAddr: server.URL}

	got, err := client.QueryInstant(context.Background(), `sum(rate({app="api"}[1m]))`, 1700000000123)

	require.NoError(t, err)
	require.Equal(t, []interface{}{float64(1700000000), "3"}, got.Data.Result[0].Value)
}

func TestLabelNames_SortsFiltersAndLimitsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/labels", r.URL.Path)
		assert.Equal(t, `{app="api"}`, r.URL.Query().Get("query"))
		assert.Equal(t, "1700000000000000000", r.URL.Query().Get("start"))
		assert.Equal(t, "1700000060000000000", r.URL.Query().Get("end"))
		_, _ = w.Write([]byte(`{"status":"success","data":["zone","host_name","app","host_id"]}`))
	}))
	defer server.Close()
	client := &Loki{LokiAddr: server.URL}

	got, err := client.LabelNames(context.Background(), `{app="api"}`, 1700000000, 1700000060, "host", 1)

	require.NoError(t, err)
	require.Equal(t, []string{"host_id"}, got)
}

func TestLabelValues_ReturnsFilteredValueObjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/label/host/values", r.URL.Path)
		assert.Empty(t, r.URL.Query())
		_, _ = w.Write([]byte(`{"status":"success","data":["web-2","api-2","api-1"]}`))
	}))
	defer server.Close()
	client := &Loki{LokiAddr: server.URL}

	got, err := client.LabelValues(context.Background(), "", 0, 0, "host", "api", 0)

	require.NoError(t, err)
	require.Equal(t, []LabelValue{{Value: "api-1"}, {Value: "api-2"}}, got)
}

func TestParsedFields_DeduplicatesAndSortsFieldValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "500", r.URL.Query().Get("limit"))
		assert.Equal(t, "backward", r.URL.Query().Get("direction"))
		_, _ = w.Write([]byte(`{
  "status": "success",
  "data": {
    "result": [
      {
        "stream": {
          "host": "b",
          "level": "warn",
          "": "ignored"
        }
      },
      {
        "stream": {
          "host": "a",
          "level": "warn"
        }
      },
      {
        "stream": {
          "host": "",
          "level": "error"
        }
      }
    ]
  }
}`))
	}))
	defer server.Close()
	client := &Loki{LokiAddr: server.URL}

	got, err := client.ParsedFields(context.Background(), `{app="api"} | json`, 1, 2, 999)

	require.NoError(t, err)
	require.Equal(t, []FieldMeta{
		{Field: "host", Values: []string{"a", "b"}}, {Field: "level", Values: []string{"error", "warn"}},
	}, got)
}

func TestLokiEndpoints_RejectInvalidResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "HTTP failure", status: http.StatusUnauthorized, body: "denied", want: "status=401 body=denied"},
		{name: "invalid JSON", status: http.StatusOK, body: `{`, want: "decode"},
		{name: "API failure", status: http.StatusOK, body: `{"status":"error","error":"bad query"}`, want: "bad query"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client := &Loki{LokiAddr: server.URL}
			calls := []struct {
				name string
				run  func() error
			}{
				{
					name: "range",
					run:  func() error { _, err := client.QueryRange(context.Background(), "q", 1, 2, "", 0, ""); return err },
				},
				{
					name: "instant",
					run:  func() error { _, err := client.QueryInstant(context.Background(), "q", 1); return err },
				},
				{
					name: "labels",
					run:  func() error { _, err := client.LabelNames(context.Background(), "", 0, 0, "", 0); return err },
				},
				{
					name: "values",
					run:  func() error { _, err := client.LabelValues(context.Background(), "", 0, 0, "host", "", 0); return err },
				},
			}
			for _, call := range calls {
				t.Run(call.name, func(t *testing.T) {
					require.ErrorContains(t, call.run(), tt.want)
				})
			}
		})
	}
}

func TestDoRequest_PostsFormBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "a & b", r.PostForm.Get("query"))
		_, _ = w.Write([]byte("accepted"))
	}))
	defer server.Close()
	client := &Loki{LokiAddr: server.URL}

	got, err := client.doRequest(context.Background(), http.MethodPost, "/api/v1/query", url.Values{"query": {"a & b"}})

	require.NoError(t, err)
	require.Equal(t, "accepted", string(got))
}

func TestInitHTTPClient_AppliesTransportSettings(t *testing.T) {
	client := &Loki{Timeout: 1500, MaxIdleConnsPerHost: 7, LokiTls: LokiTLS{SkipTlsVerify: true}}

	require.NoError(t, client.InitHTTPClient())

	require.Equal(t, 1500*time.Millisecond, client.HTTPClient.Timeout)
	transport := client.HTTPClient.Transport.(*http.Transport)
	require.Equal(t, 7, transport.MaxIdleConnsPerHost)
	require.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestParseTimestampNanos_AcceptsEpochUnitsAndRFC3339(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int64
		ok    bool
	}{
		{name: "seconds integer", input: int(1700000000), want: 1700000000000000000, ok: true},
		{name: "milliseconds", input: int64(1700000000123), want: 1700000000123000000, ok: true},
		{name: "microseconds", input: int64(1700000000123456), want: 1700000000123456000, ok: true},
		{name: "fractional seconds", input: "1700000000.5", want: 1700000000500000000, ok: true},
		{name: "RFC3339", input: "2023-11-14T22:13:20.123456789Z", want: 1700000000123456789, ok: true},
		{name: "invalid", input: "yesterday"},
		{name: "nil", input: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseTimestampNanos(tt.input)

			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}
