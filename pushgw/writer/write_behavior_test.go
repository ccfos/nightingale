package writer

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func cleanupWriteMetrics(t *testing.T) string {
	t.Helper()
	key := t.Name()
	t.Cleanup(func() {
		pstat.CounterWriteTotal.DeleteLabelValues(key)
		pstat.CounterWriteErrorTotal.DeleteLabelValues(key)
		pstat.ForwardDuration.DeleteLabelValues(key)
	})
	return key
}

func TestProcess_ChangesLabelCase(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   string
	}{
		{name: "lower", action: Lowercase, want: "api:west"},
		{name: "upper", action: Uppercase, want: "API:WEST"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []prompb.Label{{Name: "job", Value: "API"}, {Name: "zone", Value: "West"}}
			cfg := &pconf.RelabelConfig{
				Action: tt.action, SourceLabels: model.LabelNames{"job", "zone"},
				Separator: ":", TargetLabel: "route",
			}

			got := Process(input, cfg)

			require.ElementsMatch(t, []prompb.Label{
				{Name: "job", Value: "API"}, {Name: "zone", Value: "West"}, {Name: "route", Value: tt.want},
			}, got)
			require.Equal(t, "API", input[0].Value)
		})
	}
}

func TestProcess_HashModAssignsStableShard(t *testing.T) {
	input := []prompb.Label{{Name: "job", Value: "hello"}}
	cfg := &pconf.RelabelConfig{Action: HashMod, SourceLabels: model.LabelNames{"job"}, Modulus: 10, TargetLabel: "shard"}

	got := Process(input, cfg)

	// MD5("hello") ends in b9719d911017c592; its unsigned 64-bit value modulo 10 is 2.
	require.ElementsMatch(t, []prompb.Label{{Name: "job", Value: "hello"}, {Name: "shard", Value: "2"}}, got)
}

func TestProcess_KeepDropsNonmatchingSeries(t *testing.T) {
	cfg := &pconf.RelabelConfig{Action: Keep, SourceLabels: model.LabelNames{"env"}, Regex: "^prod$"}

	kept := Process([]prompb.Label{{Name: "env", Value: "prod"}}, cfg)
	dropped := Process([]prompb.Label{{Name: "env", Value: "dev"}}, cfg)

	require.Equal(t, []prompb.Label{{Name: "env", Value: "prod"}}, kept)
	require.Nil(t, dropped)
}

func TestProcess_UsesPrecompiledRegex(t *testing.T) {
	input := []prompb.Label{{Name: "env", Value: "prod"}}
	cfg := &pconf.RelabelConfig{
		Action: Keep, SourceLabels: model.LabelNames{"env"}, Regex: "[",
		RegexCompiled: regexp.MustCompile("^prod$"),
	}

	got := Process(input, cfg)

	require.Equal(t, input, got)
}

func TestProcess_InvalidRegexPreservesLabels(t *testing.T) {
	input := []prompb.Label{{Name: "env", Value: "prod"}}

	got := Process(input, &pconf.RelabelConfig{Action: Drop, Regex: "["})

	require.Equal(t, input, got)
}

func TestProcess_ReplaceWithoutMatchPreservesTarget(t *testing.T) {
	input := []prompb.Label{{Name: "job", Value: "api"}, {Name: "route", Value: "original"}}
	cfg := &pconf.RelabelConfig{
		Action: Replace, SourceLabels: model.LabelNames{"job"}, Regex: "^web-(.+)$",
		TargetLabel: "route", Replacement: "$1",
	}

	got := Process(input, cfg)

	require.ElementsMatch(t, input, got)
}

func TestRelabel_RemovesDroppedSeriesAndKeepsSamples(t *testing.T) {
	input := []prompb.TimeSeries{
		{Labels: []prompb.Label{{Name: "env", Value: "dev"}}, Samples: []prompb.Sample{{Value: 1, Timestamp: 1000}}},
		{Labels: []prompb.Label{{Name: "env", Value: "prod"}}, Samples: []prompb.Sample{{Value: 2, Timestamp: 2000}}},
	}
	rules := []*pconf.RelabelConfig{{Action: Keep, SourceLabels: model.LabelNames{"env"}, Regex: "^prod$"}}

	got := Relabel(input, rules)

	require.Equal(t, input[1:], got)
}

func TestFilterNaNSamples_PreservesValidSamplesAndEmptySeries(t *testing.T) {
	input := []prompb.TimeSeries{
		{
			Labels: []prompb.Label{{Name: "job", Value: "api"}},
			Samples: []prompb.Sample{
				{Value: math.NaN(), Timestamp: 1000},
				{Value: 2, Timestamp: 2000},
				{Value: math.NaN(), Timestamp: 3000},
				{Value: 4, Timestamp: 4000},
			},
		},
		{Labels: []prompb.Label{{Name: "job", Value: "empty"}}, Samples: []prompb.Sample{{Value: math.NaN()}}},
	}

	got := filterNaNSamples(input)

	require.Len(t, got, 2)
	require.Equal(t, []prompb.Sample{{Value: 2, Timestamp: 2000}, {Value: 4, Timestamp: 4000}}, got[0].Samples)
	require.Equal(t, []prompb.Label{{Name: "job", Value: "empty"}}, got[1].Labels)
	require.Empty(t, got[1].Samples)
}

func TestFilterNaNSamples_PreservesFiniteInput(t *testing.T) {
	input := []prompb.TimeSeries{{Samples: []prompb.Sample{{Value: 1, Timestamp: 2000}}}}

	got := filterNaNSamples(input)

	require.Equal(t, input, got)
	require.Nil(t, filterNaNSamples(nil))
}

func TestBeforeWrite_JSONOmitsNaNSamples(t *testing.T) {
	key := cleanupWriteMetrics(t)
	input := []prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "job", Value: "api"}},
		Samples: []prompb.Sample{{Value: math.NaN(), Timestamp: 1000}, {Value: 3, Timestamp: 2000}},
	}}

	encoded, err := beforeWrite(key, input, false, "json")

	require.NoError(t, err)
	var got []prompb.TimeSeries
	require.NoError(t, json.Unmarshal(encoded, &got))
	require.Equal(t, []prompb.Sample{{Value: 3, Timestamp: 2000}}, got[0].Samples)
	require.Equal(t, input[0].Labels, got[0].Labels)
}

func TestBeforeWrite_ProtoPreservesSamples(t *testing.T) {
	key := cleanupWriteMetrics(t)
	input := []prompb.TimeSeries{{Samples: []prompb.Sample{{Value: 2, Timestamp: 1700000000000}}}}

	encoded, err := beforeWrite(key, input, false, "proto")

	require.NoError(t, err)
	var got prompb.WriteRequest
	require.NoError(t, proto.Unmarshal(encoded, &got))
	require.Equal(t, input, got.Timeseries)
}

func TestWriterWrite_PostsRelabeledRemoteWrite(t *testing.T) {
	key := cleanupWriteMetrics(t)
	requests := make(chan *http.Request, 1)
	bodies := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		requests <- r.Clone(r.Context())
		bodies <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	w := WriterType{
		Client: client, RetryCount: 1,
		Opts: pconf.WriterOptions{
			Url: server.URL + "/write", BasicAuthUser: "user", BasicAuthPass: "secret",
			Headers: []string{"Host", "metrics.internal", "X-Tenant", "configured"},
			WriteRelabels: []*pconf.RelabelConfig{
				{Action: Keep, SourceLabels: model.LabelNames{"env"}, Regex: "^prod$"},
				{Action: Replace, TargetLabel: "cluster", Replacement: "west"},
			},
		},
	}
	input := []prompb.TimeSeries{
		{Labels: []prompb.Label{{Name: "env", Value: "dev"}}, Samples: []prompb.Sample{{Value: 1, Timestamp: 1000}}},
		{Labels: []prompb.Label{{Name: "env", Value: "prod"}}, Samples: []prompb.Sample{{Value: 2, Timestamp: 2000}}},
	}

	w.Write(key, input, map[string]string{"X-Request-ID": "request-1"})

	require.Len(t, requests, 1)
	request := <-requests
	require.Equal(t, http.MethodPost, request.Method)
	require.Equal(t, "/write", request.URL.Path)
	require.Equal(t, "metrics.internal", request.Host)
	require.Equal(t, "snappy", request.Header.Get("Content-Encoding"))
	require.Equal(t, "application/x-protobuf", request.Header.Get("Content-Type"))
	require.Equal(t, "n9e", request.Header.Get("User-Agent"))
	require.Equal(t, "0.1.0", request.Header.Get("X-Prometheus-Remote-Write-Version"))
	require.Equal(t, "configured", request.Header.Get("X-Tenant"))
	require.Equal(t, "request-1", request.Header.Get("X-Request-ID"))
	user, pass, ok := request.BasicAuth()
	require.True(t, ok)
	require.Equal(t, "user", user)
	require.Equal(t, "secret", pass)
	decoded, err := snappy.Decode(nil, <-bodies)
	require.NoError(t, err)
	var got prompb.WriteRequest
	require.NoError(t, proto.Unmarshal(decoded, &got))
	require.Len(t, got.Timeseries, 1)
	require.ElementsMatch(t, []prompb.Label{
		{Name: "env", Value: "prod"}, {Name: "cluster", Value: "west"},
	}, got.Timeseries[0].Labels)
	require.Equal(t, input[1].Samples, got.Timeseries[0].Samples)
}

func TestWriterWrite_DoesNotPostEmptyOrDroppedBatch(t *testing.T) {
	key := cleanupWriteMetrics(t)
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	w := WriterType{
		Client: client, RetryCount: 1,
		Opts: pconf.WriterOptions{
			Url: server.URL, WriteRelabels: []*pconf.RelabelConfig{{Action: Drop, Regex: ".*"}},
		},
	}
	input := []prompb.TimeSeries{{Labels: []prompb.Label{{Name: "job", Value: "api"}}}}

	w.Write(key, nil)
	w.Write(key, input)

	require.Empty(t, requests)
}

func TestWriterPost_FailsOverAfterServerError(t *testing.T) {
	paths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		if r.URL.Path == "/failed" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	w := WriterType{Client: client, Opts: pconf.WriterOptions{Url: server.URL + "/failed," + server.URL + "/ok"}}

	err = w.Post([]byte("payload"))

	require.NoError(t, err)
	require.Len(t, paths, 2)
	require.Equal(t, "/failed", <-paths)
	require.Equal(t, "/ok", <-paths)
}

func TestWriterPost_ReturnsRequestBuildError(t *testing.T) {
	w := WriterType{Opts: pconf.WriterOptions{Url: "://bad"}}

	err := w.Post([]byte("payload"))

	// No request could be built for any URL, so nothing was written.
	require.ErrorContains(t, err, "missing protocol scheme")
}

func TestWriterPost_SkipsInvalidURLAndUsesNextOne(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	w := WriterType{Client: client, Opts: pconf.WriterOptions{Url: "://bad," + server.URL}}

	err = w.Post([]byte("payload"))

	require.NoError(t, err)
}

func TestWriterPost_ReturnsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "try later")
	}))
	t.Cleanup(server.Close)
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	w := WriterType{Client: client, Opts: pconf.WriterOptions{Url: server.URL}}

	err = w.Post([]byte("payload"))

	require.ErrorContains(t, err, "503")
	require.ErrorContains(t, err, "try later")
}
