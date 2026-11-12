package writer

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/fasttime"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func TestWriterPost_FailsOverAfterIncompleteResponse(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		requests <- r.URL.Path + ":" + string(body)
		if r.URL.Path == "/truncated" {
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, "short")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	w := WriterType{Client: client, Opts: pconf.WriterOptions{Url: server.URL + "/truncated," + server.URL + "/ok"}}

	err = w.Post([]byte("same payload"))

	require.NoError(t, err)
	require.Len(t, requests, 2)
	require.Equal(t, "/truncated:same payload", <-requests)
	require.Equal(t, "/ok:same payload", <-requests)
}

func TestWritersInit_RejectsInvalidKafkaConfigurationWithoutRegisteringBackend(t *testing.T) {
	tests := []struct {
		name      string
		options   pconf.KafkaWriterOptions
		wantError string
	}{
		{
			name:      "unknown producer with valid version",
			options:   pconf.KafkaWriterOptions{Typ: "invalid", Version: "2.1.0", Timeout: 2},
			wantError: "unknown producer type: invalid",
		},
		{
			name:      "unknown producer with invalid version",
			options:   pconf.KafkaWriterOptions{Typ: "invalid", Version: "invalid"},
			wantError: "unknown producer type: invalid",
		},
		{
			name:      "default producer without brokers",
			options:   pconf.KafkaWriterOptions{},
			wantError: "at least one broker address",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := &WritersType{
				pushgw:   pconf.Pushgw{KafkaWriters: []pconf.KafkaWriterOptions{tc.options}},
				backends: make(map[string]Writer), pushConcurrency: make(map[string]*atomic.Int64),
			}

			err := ws.Init()

			require.ErrorContains(t, err, tc.wantError)
			require.Empty(t, ws.backends)
			require.Empty(t, ws.pushConcurrency)
			require.Equal(t, int64(0), ws.AllQueueLen.Load())
		})
	}
}

func TestPushSample_FullExistingQueuePreservesPendingSample(t *testing.T) {
	key := t.Name()
	t.Cleanup(func() { pstat.CounterPushQueueErrorTotal.DeleteLabelValues(key) })
	queue := &IdentQueue{list: NewSafeListLimited(1)}
	first := prompb.TimeSeries{Samples: []prompb.Sample{{Value: 1}}}
	require.True(t, queue.list.PushFront(first))
	ws := &WritersType{queues: map[string]*IdentQueue{key: queue}}
	before := testutil.ToFloat64(pstat.CounterPushQueueErrorTotal.WithLabelValues(key))

	err := ws.PushSample(key, prompb.TimeSeries{Samples: []prompb.Sample{{Value: 2}}})

	require.NoError(t, err)
	require.Equal(t, []prompb.TimeSeries{first}, queue.list.PopBack(2))
	require.Equal(t, before+1, testutil.ToFloat64(pstat.CounterPushQueueErrorTotal.WithLabelValues(key)))
}

func TestCleanExpiredQueues_RemovesNilEntryAndKeepsLiveQueue(t *testing.T) {
	live := &IdentQueue{list: NewSafeListLimited(1), closeCh: make(chan struct{})}
	live.ts.Store(100)
	ws := &WritersType{queues: map[string]*IdentQueue{"missing": nil, "live": live}}

	ws.cleanExpiredQueues(200)

	require.Equal(t, map[string]*IdentQueue{"live": live}, ws.queues)
	select {
	case <-live.closeCh:
		t.Fatal("live queue was closed")
	default:
	}
}

func TestMarshalAndSnappyEncode_PreservesPayloadLargerThanPoolLimit(t *testing.T) {
	input := []prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "payload", Value: strings.Repeat("x", maxPooledBufCap+1)}},
		Samples: []prompb.Sample{{Value: 7, Timestamp: 1000}},
	}}

	encoded, release, err := marshalAndSnappyEncode(input)
	require.NoError(t, err)
	t.Cleanup(release)
	decoded, err := snappy.Decode(nil, encoded)

	require.NoError(t, err)
	var got prompb.WriteRequest
	require.NoError(t, got.Unmarshal(decoded))
	require.Equal(t, input, got.Timeseries)
}

func TestWriterWrite_ForceTimestampPreservesLaterSamples(t *testing.T) {
	key := cleanupWriteMetrics(t)
	bodies := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		bodies <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	w := WriterType{Client: client, ForceUseServerTS: true, RetryCount: 1, Opts: pconf.WriterOptions{Url: server.URL}}
	input := []prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "__name__", Value: "cpu"}},
		Samples: []prompb.Sample{{Value: 1, Timestamp: -1}, {Value: 2, Timestamp: 2000}},
	}}
	before := int64(fasttime.UnixTimestamp()) * 1000

	w.Write(key, input)

	after := int64(fasttime.UnixTimestamp()) * 1000
	require.Len(t, bodies, 1)
	decoded, err := snappy.Decode(nil, <-bodies)
	require.NoError(t, err)
	var got prompb.WriteRequest
	require.NoError(t, got.Unmarshal(decoded))
	require.Len(t, got.Timeseries, 1)
	require.Len(t, got.Timeseries[0].Samples, 2)
	require.GreaterOrEqual(t, got.Timeseries[0].Samples[0].Timestamp, before)
	require.LessOrEqual(t, got.Timeseries[0].Samples[0].Timestamp, after)
	require.Equal(t, float64(1), got.Timeseries[0].Samples[0].Value)
	require.Equal(t, prompb.Sample{Value: 2, Timestamp: 2000}, got.Timeseries[0].Samples[1])
}

func TestProcess_ReplaceWithoutTargetPreservesLabels(t *testing.T) {
	input := []prompb.Label{{Name: "job", Value: "api"}}

	got := Process(input, &pconf.RelabelConfig{Action: Replace, Replacement: "ignored"})

	require.Equal(t, input, got)
}

func TestProcess_MissingSourceLabelExpandsAsEmptyString(t *testing.T) {
	cfg := &pconf.RelabelConfig{
		Action: Replace, SourceLabels: model.LabelNames{"missing"}, Regex: "^$", TargetLabel: "route", Replacement: "default",
	}

	got := Process([]prompb.Label{{Name: "job", Value: "api"}}, cfg)

	require.ElementsMatch(t, []prompb.Label{{Name: "job", Value: "api"}, {Name: "route", Value: "default"}}, got)
}

func TestProcess_LabelDropCanRemoveAllLabels(t *testing.T) {
	got := Process([]prompb.Label{{Name: "job", Value: "api"}}, &pconf.RelabelConfig{Action: LabelDrop, Regex: ".*"})

	require.Empty(t, got)
	require.NotNil(t, got)
}

func TestProcess_DropIfEqualWithOneSourcePreservesSeries(t *testing.T) {
	input := []prompb.Label{{Name: "job", Value: "api"}}
	cfg := &pconf.RelabelConfig{Action: DropIfEqual, SourceLabels: model.LabelNames{"job"}}

	got := Process(input, cfg)

	require.Equal(t, input, got)
}
