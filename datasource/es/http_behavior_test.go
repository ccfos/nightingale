package es

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

func newHTTPTestElasticsearch(t *testing.T, handler http.HandlerFunc) *Elasticsearch {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := &Elasticsearch{
		Nodes: []string{server.URL}, Version: "7.17.0", Timeout: 1000, MaxShard: 3,
		Basic: BasicAuth{Username: "reader", Password: "secret"}, Headers: map[string]string{"X-Tenant": "team"},
	}
	require.NoError(t, client.InitClient())
	t.Cleanup(client.Client.Stop)
	return client
}

func TestElasticsearch_QueryIndicesSendsAuthAndParsesNames(t *testing.T) {
	client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/_all/_settings", r.URL.Path)
		assert.Equal(t, "team", r.Header.Get("X-Tenant"))
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "reader", user)
		assert.Equal(t, "secret", pass)
		_, _ = w.Write([]byte(`{"logs-b":{"settings":{}},"logs-a":{"settings":{}}}`))
	})

	got, err := client.QueryIndices()

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"logs-a", "logs-b"}, got)
}

func TestElasticsearch_QueryFieldsFiltersAndDeduplicatesMappings(t *testing.T) {
	tests := []struct {
		name    string
		version string
		body    string
	}{
		{
			name: "ES7", version: "7.17.0",
			body: `{
  "logs-a": {
    "mappings": {
      "host": {
        "mapping": {
          "host": {
            "type": "keyword"
          }
        }
      },
      "message": {
        "mapping": {
          "message": {
            "type": "text"
          }
        }
      }
    }
  },
  "logs-b": {
    "mappings": {
      "host": {
        "mapping": {
          "host": {
            "type": "keyword"
          }
        }
      },
      "latency": {
        "mapping": {
          "latency": {
            "type": "double"
          }
        }
      }
    }
  }
}`,
		},
		{
			name: "ES6", version: "6.8.0",
			body: `{
  "logs-a": {
    "mappings": {
      "doc": {
        "host": {
          "mapping": {
            "host": {
              "type": "keyword"
            }
          }
        },
        "message": {
          "mapping": {
            "message": {
              "type": "text"
            }
          }
        },
        "latency": {
          "mapping": {
            "latency": {
              "type": "double"
            }
          }
        }
      }
    }
  }
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/logs-*/_mapping/_all/field/*", r.URL.Path)
				assert.Equal(t, "true", r.URL.Query().Get("ignore_unavailable"))
				_, _ = w.Write([]byte(tt.body))
			})
			client.Version = tt.version

			got, err := client.QueryFields([]string{"logs-*"})

			require.NoError(t, err)
			require.Equal(t, []string{"host", "latency"}, got)
		})
	}
}

func TestElasticsearch_QueryFieldValueBuildsAggregationAndSkipsNumericKeys(t *testing.T) {
	client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
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
		_, _ = w.Write([]byte(`{"aggregations":{"distinct":{"buckets":[{"key":"a","doc_count":2},{"key":42,"doc_count":1}]}}}`))
	})

	got, err := client.QueryFieldValue([]string{"logs-*"}, "host", "service:api")

	require.NoError(t, err)
	require.Equal(t, []string{"a"}, got)
}

func TestElasticsearch_QueryLogSendsPaginationAndReturnsHits(t *testing.T) {
	client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/logs-*/_search", r.URL.Path)
		assert.Equal(t, "3", r.URL.Query().Get("max_concurrent_shard_requests"))
		var body map[string]interface{}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, float64(5), body["size"])
		assert.Equal(t, float64(10), body["from"])
		assert.Equal(t, true, body["track_total_hits"])
		_, _ = w.Write([]byte(`{
  "hits": {
    "total": {
      "value": 12,
      "relation": "eq"
    },
    "hits": [
      {
        "_id": "one",
        "_index": "logs-a",
        "_source": {
          "message": "hello"
        }
      }
    ]
  }
}`))
	})

	got, total, err := client.QueryLog(context.Background(), eslike.Query{
		Index: "logs-*", Start: 1700000000, End: 1700000060, DateField: "ts", Limit: 5, P: 10, Timeout: 2,
	})

	require.NoError(t, err)
	require.Equal(t, int64(12), total)
	require.Len(t, got, 1)
	hit := got[0].(*elastic.SearchHit)
	require.Equal(t, "one", hit.Id)
	require.JSONEq(t, `{"message":"hello"}`, string(hit.Source))
}

func TestElasticsearch_SearchLogOmitsIndexOptionsForPointInTime(t *testing.T) {
	client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/_search", r.URL.Path)
		assert.Empty(t, r.URL.Query().Get("ignore_unavailable"))
		var body map[string]interface{}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, map[string]interface{}{"id": "pit-id", "keep_alive": "1m"}, body["pit"])
		_, _ = w.Write([]byte(`{"hits":{"hits":[]}}`))
	})
	source := elastic.NewSearchSource().PointInTime(elastic.NewPointInTimeWithKeepAlive("pit-id", "1m"))

	got, err := client.searchLog(context.Background(), nil, source, 2, 3)

	require.NoError(t, err)
	require.Empty(t, got.Hits.Hits)
}

func TestElasticsearch_QueryMapDataConvertsSourceScalars(t *testing.T) {
	client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/logs-*/_search", r.URL.Path)
		_, _ = w.Write([]byte(`{
  "hits": {
    "total": {
      "value": 2,
      "relation": "eq"
    },
    "hits": [
      {
        "_source": {
          "host": "a",
          "value": 2.5,
          "healthy": true
        }
      },
      {
        "_source": {
          "host": "b",
          "value": 3
        }
      }
    ]
  }
}`))
	})

	got, err := client.QueryMapData(context.Background(), eslike.Query{
		Index: "logs-*", Start: 1700000000, End: 1700000060, DateField: "ts", Limit: 2,
	})

	require.NoError(t, err)
	require.Equal(t, []map[string]string{
		{"host": "a", "value": "2.5", "healthy": "true"}, {"host": "b", "value": "3"},
	}, got)
}

func TestElasticsearch_MetadataQueriesPropagateHTTPFailure(t *testing.T) {
	client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"bad_request","reason":"invalid index"},"status":400}`))
	})

	_, indicesErr := client.QueryIndices()
	_, fieldsErr := client.QueryFields([]string{"logs"})
	_, valuesErr := client.QueryFieldValue([]string{"logs"}, "host", "")
	_, mapErr := client.QueryMapData(context.Background(), eslike.Query{Index: "logs", Start: 1, End: 2})

	require.ErrorContains(t, indicesErr, "failed to get index names")
	require.ErrorContains(t, fieldsErr, "failed to get field mappings")
	require.ErrorContains(t, valuesErr, "failed to query field values")
	require.ErrorContains(t, mapErr, "failed to query log data")
}

func TestElasticsearch_QueryDataBuildsHistogramAndConvertsCounts(t *testing.T) {
	client := newHTTPTestElasticsearch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/logs/_search", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("ignore_unavailable"))
		assert.Equal(t, "5", r.URL.Query().Get("max_concurrent_shard_requests"))
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
