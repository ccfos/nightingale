package prom

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func localPromAPI(t *testing.T, handler http.HandlerFunc, opts ClientOptions) API {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	return NewAPI(client, opts)
}

func TestQuery_PostsExpressionAndParsesVector(t *testing.T) {
	var form url.Values
	var method, path, host, auth, tenant string
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form, method, path = r.PostForm, r.Method, r.URL.Path
		host, auth, tenant = r.Host, r.Header.Get("Authorization"), r.Header.Get("X-Tenant")
		_, _ = io.WriteString(w, `{"status":"success","warnings":["partial data"],`+
			`"data":{"resultType":"vector","result":[{"metric":{"job":"api"},"value":[1700000000.125,"2.5"]}]}}`)
	}, ClientOptions{
		BasicAuthUser: "reader", BasicAuthPass: "secret",
		Headers: []string{"Host", "metrics.local", "X-Tenant", "team-a"},
	})

	value, warnings, err := client.Query(context.Background(), `up{job="api"}`, time.Unix(1700000000, 125000000))

	require.NoError(t, err)
	require.Equal(t, http.MethodPost, method)
	require.Equal(t, "/api/v1/query", path)
	require.Equal(t, `up{job="api"}`, form.Get("query"))
	require.Equal(t, "1700000000.125", form.Get("time"))
	require.Equal(t, "metrics.local", host)
	require.Equal(t, "Basic cmVhZGVyOnNlY3JldA==", auth)
	require.Equal(t, "team-a", tenant)
	require.Equal(t, Warnings{"partial data"}, warnings)
	require.Equal(t, model.Vector{&model.Sample{
		Metric: model.Metric{"job": "api"}, Timestamp: model.Time(1700000000125), Value: model.SampleValue(2.5),
	}}, value)
}

func TestQueryRange_FallsBackToGetAndParsesMatrix(t *testing.T) {
	var methods []string
	var params url.Values
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		params = r.URL.Query()
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"matrix",`+
			`"result":[{"metric":{"job":"api"},"values":[[100,"1"],[101.5,"3"]]}]}}`)
	}, ClientOptions{})

	value, warnings, err := client.QueryRange(context.Background(), "up", Range{
		Start: time.Unix(100, 0), End: time.Unix(103, 0), Step: 1500 * time.Millisecond,
	})

	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Equal(t, []string{http.MethodPost, http.MethodGet}, methods)
	require.Equal(t, url.Values{"query": {"up"}, "start": {"100"}, "end": {"103"}, "step": {"1.5"}}, params)
	require.Equal(t, model.Matrix{&model.SampleStream{
		Metric: model.Metric{"job": "api"},
		Values: []model.SamplePair{{Timestamp: 100000, Value: 1}, {Timestamp: 101500, Value: 3}},
	}}, value)
}

func TestLabelNames_ReturnsLabelsAndWarnings(t *testing.T) {
	var path string
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = io.WriteString(w, `{"status":"success","warnings":["limited"],"data":["__name__","job"]}`)
	}, ClientOptions{})

	names, warnings, err := client.LabelNames(context.Background())

	require.NoError(t, err)
	require.Equal(t, "/api/v1/labels", path)
	require.Equal(t, []string{"__name__", "job"}, names)
	require.Equal(t, Warnings{"limited"}, warnings)
}

func TestLabelValues_EncodesRepeatedMatchers(t *testing.T) {
	var path string
	var matches []string
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		path, matches = r.URL.Path, r.URL.Query()["match[]"]
		_, _ = io.WriteString(w, `{"status":"success","data":["api","worker"]}`)
	}, ClientOptions{})

	values, _, err := client.LabelValues(context.Background(), "job", []string{"up", `errors{env="prod"}`})

	require.NoError(t, err)
	require.Equal(t, "/api/v1/label/job/values", path)
	require.Equal(t, []string{"up", `errors{env="prod"}`}, matches)
	require.Equal(t, model.LabelValues{"api", "worker"}, values)
}

func TestSeries_EncodesTimeBoundsAndParsesLabels(t *testing.T) {
	var params url.Values
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		params = r.URL.Query()
		_, _ = io.WriteString(w, `{"status":"success","data":[{"__name__":"up","job":"api"}]}`)
	}, ClientOptions{})

	series, _, err := client.Series(context.Background(), []string{"up", "errors"}, time.Unix(100, 0), time.Unix(200, 0))

	require.NoError(t, err)
	require.Equal(t, url.Values{"match[]": {"up", "errors"}, "start": {"100"}, "end": {"200"}}, params)
	require.Equal(t, []model.LabelSet{{"__name__": "up", "job": "api"}}, series)
}

func TestLabelNames_ReportsAPIAndHTTPFailures(t *testing.T) {
	tests := []struct {
		name        string
		code        int
		body        string
		wantType    ErrorType
		wantMessage string
	}{
		{name: "api error", code: 422, body: `{"status":"error","errorType":"execution","error":"bad expression"}`,
			wantType: ErrExec, wantMessage: "bad expression"},
		{name: "server error", code: 503, body: "unavailable", wantType: ErrServer, wantMessage: "server error: 503"},
		{name: "client error", code: 403, body: "denied", wantType: ErrClient, wantMessage: "client error: 403"},
		{name: "invalid json", code: 200, body: "{", wantType: ErrBadResponse},
		{name: "inconsistent status", code: 200, body: `{"status":"error"}`,
			wantType: ErrBadResponse, wantMessage: "inconsistent body for response code"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
				_, _ = io.WriteString(w, tt.body)
			}, ClientOptions{})

			_, _, err := client.LabelNames(context.Background())

			var apiErr *Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, tt.wantType, apiErr.Type)
			if tt.wantMessage != "" {
				require.Equal(t, tt.wantMessage, apiErr.Msg)
			}
			require.Contains(t, apiErr.Error(), string(tt.wantType))
		})
	}
}

func TestRules_DecodesAlertingAndRecordingRules(t *testing.T) {
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"success","data":{"groups":[{"name":"main","file":"rules.yml","interval":30,`+
			`"rules":[{"type":"alerting","name":"Down","query":"up == 0","duration":60,"health":"ok"},`+
			`{"type":"recording","name":"job:up","query":"sum(up)","health":"ok"}]}]}}`)
	}, ClientOptions{})

	result, err := client.Rules(context.Background())

	require.NoError(t, err)
	require.Len(t, result.Groups, 1)
	require.Equal(t, "main", result.Groups[0].Name)
	require.Equal(t, []interface{}{
		AlertingRule{Name: "Down", Query: "up == 0", Duration: 60, Health: RuleHealthGood},
		RecordingRule{Name: "job:up", Query: "sum(up)", Health: RuleHealthGood},
	}, []interface{}(result.Groups[0].Rules))
}

func TestQueryResult_RejectsUnsupportedOrMalformedSamples(t *testing.T) {
	for _, body := range []string{
		`{"resultType":"string","result":[1,"value"]}`,
		`{"resultType":"matrix","result":[{"metric":{},"values":[[1,"bad"]]}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			var result QueryResult

			err := result.UnmarshalJSON([]byte(body))

			require.Error(t, err)
		})
	}
}

func TestRangeValidate_NormalizesStepWithinPointLimit(t *testing.T) {
	tests := []struct {
		name  string
		span  time.Duration
		step  time.Duration
		valid bool
	}{
		{name: "default step", span: time.Hour, valid: true},
		{name: "subsecond step", span: 24 * time.Hour, step: time.Millisecond, valid: true},
		{name: "large range", span: 365 * 24 * time.Hour, step: time.Second, valid: true},
		{name: "reversed bounds", span: -time.Hour, valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Range{Start: time.Unix(0, 0), End: time.Unix(0, 0).Add(tt.span), Step: tt.step}

			valid := r.Validate()

			require.Equal(t, tt.valid, valid)
			if valid {
				require.Positive(t, r.Step)
				require.LessOrEqual(t, r.End.Sub(r.Start)/r.Step, time.Duration(MaxPoints))
			}
		})
	}
}

func TestWriterWrite_PostsSnappyProtobufWithHeaders(t *testing.T) {
	var received prompb.WriteRequest
	var headers http.Header
	var path, auth string
	var decodeErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers, path, auth = r.Header.Clone(), r.URL.Path, r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err == nil {
			body, err = snappy.Decode(nil, body)
		}
		if err == nil {
			err = proto.Unmarshal(body, &received)
		}
		decodeErr = err
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	writer := NewWriter(client, ClientOptions{
		Url: server.URL + "/write", BasicAuthUser: "writer", BasicAuthPass: "secret", Headers: []string{"X-Tenant", "team-a"},
	})
	series := []prompb.TimeSeries{{
		Labels: []prompb.Label{{Name: "__name__", Value: "up"}}, Samples: []prompb.Sample{{Timestamp: 1000, Value: 2}},
	}}

	err = writer.Write(series, map[string]string{"X-Request-ID": "request-1"})

	require.NoError(t, err)
	require.NoError(t, decodeErr)
	require.Equal(t, series, received.Timeseries)
	require.Equal(t, "/write", path)
	require.Equal(t, "snappy", headers.Get("Content-Encoding"))
	require.Equal(t, "application/x-protobuf", headers.Get("Content-Type"))
	require.Equal(t, "0.1.0", headers.Get("X-Prometheus-Remote-Write-Version"))
	require.Equal(t, "team-a", headers.Get("X-Tenant"))
	require.Equal(t, "request-1", headers.Get("X-Request-ID"))
	require.Equal(t, "Basic d3JpdGVyOnNlY3JldA==", auth)
}

func TestWriterPost_ReturnsHTTPErrorWithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "invalid samples")
	}))
	defer server.Close()
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	writer := NewWriter(client, ClientOptions{Url: server.URL})

	err = writer.Post([]byte("payload"))

	require.ErrorContains(t, err, "400")
	require.ErrorContains(t, err, "invalid samples")
}

func TestQueryResult_ParsesScalar(t *testing.T) {
	var result QueryResult

	err := json.Unmarshal([]byte(`{"resultType":"scalar","result":[123.5,"7"]}`), &result)

	require.NoError(t, err)
	require.Equal(t, &model.Scalar{Timestamp: 123500, Value: 7}, result.v)
}

func TestConfig_ReturnsYAML(t *testing.T) {
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"success","data":{"yaml":"global:\n  scrape_interval: 30s\n"}}`)
	}, ClientOptions{})

	result, err := client.Config(context.Background())

	require.NoError(t, err)
	require.Equal(t, "global:\n  scrape_interval: 30s\n", result.YAML)
}

func TestSnapshot_PostsSkipHeadAndParsesSnapshotName(t *testing.T) {
	var method, path, skipHead string
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		method, path, skipHead = r.Method, r.URL.Path, r.URL.Query().Get("skip_head")
		_, _ = io.WriteString(w, `{"status":"success","data":{"name":"snapshot-1"}}`)
	}, ClientOptions{})

	result, err := client.Snapshot(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, SnapshotResult{Name: "snapshot-1"}, result)
	require.Equal(t, http.MethodPost, method)
	require.Equal(t, "/api/v1/admin/tsdb/snapshot", path)
	require.Equal(t, "true", skipHead)
}

func TestDeleteSeries_PostsMatchersAndBounds(t *testing.T) {
	var method, path string
	var params url.Values
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		method, path, params = r.Method, r.URL.Path, r.URL.Query()
		w.WriteHeader(http.StatusNoContent)
	}, ClientOptions{})

	err := client.DeleteSeries(context.Background(), []string{"up", "errors"}, time.Unix(100, 0), time.Unix(200, 0))

	require.NoError(t, err)
	require.Equal(t, http.MethodPost, method)
	require.Equal(t, "/api/v1/admin/tsdb/delete_series", path)
	require.Equal(t, url.Values{"match[]": {"up", "errors"}, "start": {"100"}, "end": {"200"}}, params)
}

func TestMetadata_EncodesMetricAndLimit(t *testing.T) {
	var params url.Values
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		params = r.URL.Query()
		_, _ = io.WriteString(w, `{"status":"success","data":{"requests":[{"type":"counter","help":"Request count","unit":""}]}}`)
	}, ClientOptions{})

	result, err := client.Metadata(context.Background(), "requests", "5")

	require.NoError(t, err)
	require.Equal(t, url.Values{"metric": {"requests"}, "limit": {"5"}}, params)
	require.Equal(t, map[string][]Metadata{
		"requests": {{Type: MetricTypeCounter, Help: "Request count"}},
	}, result)
}

func TestTargetsMetadata_EncodesTargetFilter(t *testing.T) {
	var params url.Values
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		params = r.URL.Query()
		_, _ = io.WriteString(w, `{"status":"success","data":[{"target":{"job":"api"},`+
			`"metric":"requests","type":"counter","help":"Requests"}]}`)
	}, ClientOptions{})

	result, err := client.TargetsMetadata(context.Background(), `{job="api"}`, "requests", "2")

	require.NoError(t, err)
	require.Equal(t, url.Values{"match_target": {`{job="api"}`}, "metric": {"requests"}, "limit": {"2"}}, params)
	require.Equal(t, []MetricMetadata{{
		Target: map[string]string{"job": "api"}, Metric: "requests", Type: MetricTypeCounter, Help: "Requests",
	}}, result)
}

