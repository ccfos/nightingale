package opensearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource/commons/eslike"
	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHTTPTestOpenSearch(t *testing.T, handler http.HandlerFunc) *OpenSearch {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := &OpenSearch{
		Nodes: []string{server.URL}, Version: "2.17.0", Timeout: 1000,
		Basic: BasicAuth{Username: "reader", Password: "secret"}, Headers: map[string]string{"X-Tenant": "team"},
	}
	require.NoError(t, client.InitClient())
	return client
}

func TestOpenSearch_InitDecodesSettings(t *testing.T) {
	ds, err := new(OpenSearch).Init(map[string]interface{}{
		"os.nodes": []string{"http://127.0.0.1:19200"}, "os.timeout": 1500, "os.version": "2.17.0",
		"os.basic":   map[string]interface{}{"os.user": "reader", "os.password": "secret"},
		"os.headers": map[string]string{"X-Tenant": "team"},
	})

	require.NoError(t, err)
	got := ds.(*OpenSearch)
	require.Equal(t, []string{"http://127.0.0.1:19200"}, got.Nodes)
	require.Equal(t, int64(1500), got.Timeout)
	require.Equal(t, "2.17.0", got.Version)
	require.Equal(t, BasicAuth{Username: "reader", Password: "secret"}, got.Basic)
	require.Equal(t, "team", got.Headers["X-Tenant"])
	_, err = new(OpenSearch).Init(map[string]interface{}{"os.timeout": "bad"})
	require.Error(t, err)
}

func TestOpenSearch_ValidateAppliesDefaults(t *testing.T) {
	client := &OpenSearch{Nodes: []string{"http://127.0.0.1:19200"}, Version: "2.17.0"}

	err := client.Validate(context.Background())

	require.NoError(t, err)
	require.Equal(t, int64(6000), client.Timeout)
	require.Equal(t, 10, client.MinInterval)
	require.Equal(t, 5, client.MaxShard)
}

func TestOpenSearch_ValidateRejectsInvalidSettings(t *testing.T) {
	tests := []struct {
		name   string
		config OpenSearch
		want   string
	}{
		{name: "no nodes", config: OpenSearch{Version: "2"}, want: "valid addr"},
		{name: "malformed URL", config: OpenSearch{Nodes: []string{"://bad"}, Version: "2"}, want: "parse addr"},
		{
			name:   "missing password",
			config: OpenSearch{Nodes: []string{"http://127.0.0.1"}, Version: "2", Basic: BasicAuth{Username: "reader"}},
			want:   "password is required",
		},
		{
			name:   "unsupported version",
			config: OpenSearch{Nodes: []string{"http://127.0.0.1"}, Version: "1.0"},
			want:   "version",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate(context.Background())

			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestOpenSearch_EqualDetectsConfigurationChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(*OpenSearch)
	}{
		{name: "nodes", change: func(c *OpenSearch) { c.Nodes = []string{"http://127.0.0.1:19201"} }},
		{name: "user", change: func(c *OpenSearch) { c.Basic.Username = "other" }},
		{name: "password", change: func(c *OpenSearch) { c.Basic.Password = "other" }},
		{name: "TLS", change: func(c *OpenSearch) { c.TLS.SkipTlsVerify = true }},
		{name: "timeout", change: func(c *OpenSearch) { c.Timeout++ }},
		{name: "headers", change: func(c *OpenSearch) { c.Headers = map[string]string{"X-Tenant": "other"} }},
		{name: "version", change: func(c *OpenSearch) { c.Version = "2.0" }},
		{name: "interval", change: func(c *OpenSearch) { c.MinInterval++ }},
		{name: "shards", change: func(c *OpenSearch) { c.MaxShard++ }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := &OpenSearch{Nodes: []string{"http://127.0.0.1:19200"}}
			changed := *base
			tt.change(&changed)

			require.False(t, base.Equal(&changed))
			require.True(t, base.Equal(base))
		})
	}
}

func TestOpenSearch_QueryIndicesSendsHeadersAndParsesNames(t *testing.T) {
	client := newHTTPTestOpenSearch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/_cat/indices", r.URL.Path)
		assert.Equal(t, "json", r.URL.Query().Get("format"))
		assert.Equal(t, "team", r.Header.Get("X-Tenant"))
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "reader", user)
		assert.Equal(t, "secret", pass)
		_, _ = w.Write([]byte(`[{"index":"logs-a"},{"index":"logs-b"}]`))
	})

	got, err := client.QueryIndices()

	require.NoError(t, err)
	require.Equal(t, []string{"logs-a", "logs-b"}, got)
}

func TestOpenSearch_QueryFieldsFlattensNestedMappings(t *testing.T) {
	client := newHTTPTestOpenSearch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/logs-2026/_mapping", r.URL.Path)
		_, _ = w.Write([]byte(`{
  "logs-2026": {
    "mappings": {
      "properties": {
        "host": {
          "type": "keyword"
        },
        "message": {
          "type": "text"
        },
        "http": {
          "properties": {
            "status": {
              "type": "integer"
            }
          }
        },
        "latency": {
          "type": "double"
        }
      }
    }
  }
}`))
	})

	got, err := client.QueryFields([]string{"logs-2026"})

	require.NoError(t, err)
	require.Equal(t, []string{"host", "http.status", "latency"}, got)
}

func TestOpenSearch_QueryFieldValueBuildsTermsAggregation(t *testing.T) {
	client := newHTTPTestOpenSearch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/logs-*/_search", r.URL.Path)
		var body map[string]interface{}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		raw, err := json.Marshal(body)
		assert.NoError(t, err)
		assert.JSONEq(t, `{
  "size": 0,
  "query": {
    "bool": {
      "must": {
        "query_string": {
          "query": "service:api"
        }
      }
    }
  },
  "aggregations": {
    "distinct": {
      "terms": {
        "field": "host",
        "size": 10000
      }
    }
  }
}`, string(raw))
		_, _ = w.Write([]byte(`{"aggregations":{"distinct":{"buckets":[{"key":"a","doc_count":2},{"key":"b","doc_count":1}]}}}`))
	})

	got, err := client.QueryFieldValue([]string{"logs-*"}, "host", "service:api")

	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, got)
}

func TestOpenSearch_QueryLogReturnsHitsAndTotal(t *testing.T) {
	client := newHTTPTestOpenSearch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/logs-*/_search", r.URL.Path)
		var body map[string]interface{}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, float64(5), body["size"])
		assert.Equal(t, true, body["track_total_hits"])
		_, _ = w.Write([]byte(`{
  "hits": {
    "total": {
      "value": 7,
      "relation": "eq"
    },
    "hits": [
      {
        "_id": "one",
        "_index": "logs-1",
        "_source": {
          "message": "hello"
        }
      }
    ]
  }
}`))
	})

	got, total, err := client.QueryLog(context.Background(), eslike.Query{
		Index: "logs-*", Start: 1700000000, End: 1700000060, DateField: "ts", Limit: 5,
	})

	require.NoError(t, err)
	require.Equal(t, int64(7), total)
	require.Len(t, got, 1)
	hit := got[0].(*elastic.SearchHit)
	require.Equal(t, "one", hit.Id)
	require.JSONEq(t, `{"message":"hello"}`, string(hit.Source))
}

func TestSearch_PropagatesHTTPAndJSONErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "HTTP", status: http.StatusBadRequest, body: "bad query", want: "not 2xx"},
		{name: "JSON", status: http.StatusOK, body: "not JSON", want: "invalid character"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newHTTPTestOpenSearch(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "2000ms", r.URL.Query().Get("timeout"))
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			got, err := search(context.Background(), []string{"logs"}, map[string]interface{}{"size": 1}, 2, client.Client)

			require.ErrorContains(t, err, tt.want)
			require.Nil(t, got)
		})
	}
}

func TestSearch_RejectsUnencodableSourceBeforeHTTP(t *testing.T) {
	got, err := search(context.Background(), nil, make(chan int), 0, nil)

	require.ErrorContains(t, err, "unsupported type")
	require.Nil(t, got)
}

func TestOpenSearch_QueryDataBuildsHistogramAndConvertsCounts(t *testing.T) {
	client := newHTTPTestOpenSearch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/logs/_search", r.URL.Path)
		var body map[string]interface{}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		histogram := body["aggregations"].(map[string]interface{})["ts"].(map[string]interface{})
		assert.Equal(t, "ts", histogram["date_histogram"].(map[string]interface{})["field"])
		_, _ = w.Write([]byte(`{
   "aggregations":{"ts":{"buckets":[
    {"key":1700000000000,"key_as_string":"2023-11-14T22:13:20Z","doc_count":9}
   ]}}
  }`))
	})

	got, err := client.QueryData(context.Background(), eslike.Query{
		Index: "logs", DateField: "ts", Start: 1700000000, End: 1700000060, Interval: 60, Ref: "A",
		MetricAggr: eslike.MetricAggr{Func: "count", Field: "requests"},
	})

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "A", got[0].Ref)
	require.Equal(t, [][]float64{{1700000000, 9}}, got[0].Values)
}
