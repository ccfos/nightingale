package writer

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/IBM/sarama"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func TestSafeList_PopsBatchesInFIFOOrder(t *testing.T) {
	queue := NewSafeList()
	first := prompb.TimeSeries{Samples: []prompb.Sample{{Value: 1}}}
	second := prompb.TimeSeries{Samples: []prompb.Sample{{Value: 2}}}
	queue.PushFrontBatch([]interface{}{first, "not a series", second})

	got := queue.PopBack(2)

	require.Equal(t, []prompb.TimeSeries{first}, got)
	require.Equal(t, 1, queue.Len())
	require.Equal(t, []prompb.TimeSeries{second}, queue.PopBack(10))
	require.Empty(t, queue.PopBack(10))
}

func TestSafeListLimited_RejectsBatchWhenFullAndCanBeReusedAfterClear(t *testing.T) {
	queue := NewSafeListLimited(2)
	first := prompb.TimeSeries{Samples: []prompb.Sample{{Value: 1}}}
	second := prompb.TimeSeries{Samples: []prompb.Sample{{Value: 2}}}

	require.True(t, queue.PushFrontBatch([]interface{}{first, second}))
	require.False(t, queue.PushFrontBatch([]interface{}{first}))
	require.False(t, queue.PushFront(first))
	require.Equal(t, 2, queue.Len())
	queue.RemoveAll()

	require.Zero(t, queue.Len())
	require.Empty(t, queue.PopBack(10))
	require.True(t, queue.PushFront(second))
	require.Equal(t, []prompb.TimeSeries{second}, queue.PopBack(1))
}

func TestWriterWrite_RetriesServerFailureAndCountsFailedSeries(t *testing.T) {
	key := cleanupWriteMetrics(t)
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		bodies = append(bodies, body)
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(server.Close)
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	w := WriterType{Client: client, Opts: pconf.WriterOptions{Url: server.URL}, RetryCount: 3}

	w.Write(key, []prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "__name__", Value: "up"}},
		Samples: []prompb.Sample{{Value: 3, Timestamp: 1000}},
	}})

	require.Len(t, bodies, 2)
	require.Equal(t, bodies[0], bodies[1])
	require.Equal(t, float64(1), testutil.ToFloat64(pstat.CounterWriteTotal.WithLabelValues(key)))
	require.Equal(t, float64(1), testutil.ToFloat64(pstat.CounterWriteErrorTotal.WithLabelValues(key)))
}

func TestWritersInit_RegistersHTTPBackendsAndCapsAsyncRetries(t *testing.T) {
	ws := &WritersType{
		backends: make(map[string]Writer), pushConcurrency: make(map[string]*atomic.Int64),
		pushgw: pconf.Pushgw{
			ForceUseServerTS: true,
			WriterOpt:        pconf.WriterGlobalOpt{RetryCount: 100, RetryInterval: 2},
			Writers: []pconf.WriterOptions{
				{Url: "http://127.0.0.1:19001"},
				{Url: "http://127.0.0.1:19002", AsyncWrite: true},
			},
		},
	}

	err := ws.Init()

	require.NoError(t, err)
	require.Equal(t, int64(0), ws.AllQueueLen.Load())
	require.Len(t, ws.backends, 2)
	critical := ws.backends["http://127.0.0.1:19001"].(WriterType)
	async := ws.backends["http://127.0.0.1:19002"].(WriterType)
	require.Equal(t, 100, critical.RetryCount)
	require.Equal(t, nonCriticalRetryCount, async.RetryCount)
	require.Equal(t, int64(2), async.RetryInterval)
	require.True(t, async.ForceUseServerTS)
	require.NotNil(t, async.Client)
	require.Zero(t, ws.pushConcurrency[async.Opts.Url].Load())
}

func TestWritersInit_RejectsMalformedHTTPAddress(t *testing.T) {
	ws := &WritersType{pushgw: pconf.Pushgw{Writers: []pconf.WriterOptions{{Url: "://bad"}}}}

	err := ws.Init()

	require.Error(t, err)
	require.Empty(t, ws.backends)
}

type round5Backend struct {
	write func(string, []prompb.TimeSeries)
}

func (b round5Backend) Write(key string, series []prompb.TimeSeries, _ ...map[string]string) {
	b.write(key, series)
}

func TestIsCriticalBackend_ClassifiesDeliveryPolicy(t *testing.T) {
	ws := &WritersType{backends: map[string]Writer{
		"sync": WriterType{}, "async": WriterType{Opts: pconf.WriterOptions{AsyncWrite: true}},
		"kafka": KafkaWriterType{}, "custom": round5Backend{},
	}}
	tests := []struct {
		name string
		want bool
	}{
		{name: "sync", want: true},
		{name: "async", want: false},
		{name: "kafka", want: false},
		{name: "custom", want: true},
		{name: "missing", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ws.isCriticalBackend(tt.name))
		})
	}
}

func TestWriteToNonCriticalBackend_IsolatesQuotasAndSampleBuffers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		blockedKey, availableKey := cleanupWriteMetrics(t)+"-blocked", cleanupWriteMetrics(t)
		t.Cleanup(func() { pstat.CounterWriteErrorTotal.DeleteLabelValues(blockedKey) })
		ws := &WritersType{
			backends: make(map[string]Writer), pushConcurrency: make(map[string]*atomic.Int64), pushConcurrencyLimit: 1,
		}
		calls := make(chan []prompb.TimeSeries, 1)
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		ws.Put(blockedKey, round5Backend{write: func(string, []prompb.TimeSeries) {
			t.Error("saturated backend was called")
		}})
		ws.Put(availableKey, round5Backend{write: func(key string, series []prompb.TimeSeries) {
			<-release
			calls <- series
		}})
		ws.pushConcurrency[blockedKey].Store(1)
		input := []prompb.TimeSeries{{Samples: []prompb.Sample{{Value: 1}}}}

		// Hold the available backend until the caller has changed its sample buffer.
		ws.writeToNonCriticalBackend(blockedKey, input)
		ws.writeToNonCriticalBackend(availableKey, input)
		input[0].Samples[0].Value = 9
		synctest.Wait()

		require.Equal(t, int64(1), ws.pushConcurrency[blockedKey].Load())
		require.Equal(t, float64(1), testutil.ToFloat64(pstat.CounterWriteErrorTotal.WithLabelValues(blockedKey)))
		require.Equal(t, int64(1), ws.pushConcurrency[availableKey].Load())
		release <- struct{}{}
		synctest.Wait()
		require.Equal(t, float64(1), (<-calls)[0].Samples[0].Value)
		require.Zero(t, ws.pushConcurrency[availableKey].Load())
	})
}

type round5KafkaProducer struct {
	messages []*sarama.ProducerMessage
	failures int
}

func (p *round5KafkaProducer) Send(message *sarama.ProducerMessage) error {
	p.messages = append(p.messages, message)
	if len(p.messages) <= p.failures {
		return errors.New("broker unavailable")
	}
	return nil
}

func (*round5KafkaProducer) Close() error { return nil }

func TestKafkaWriterWrite_RelabelsAndEncodesFiniteSamples(t *testing.T) {
	key := cleanupWriteMetrics(t)
	producer := &round5KafkaProducer{}
	writer := KafkaWriterType{Client: producer, RetryCount: 1, Opts: pconf.KafkaWriterOptions{
		Topic: "metrics", WriteRelabels: []*pconf.RelabelConfig{{Action: Replace, TargetLabel: "env", Replacement: "prod"}},
	}}
	input := []prompb.TimeSeries{{Samples: []prompb.Sample{{Value: math.NaN()}, {Value: 7, Timestamp: 2000}}}}

	writer.Write(key, input)

	require.Len(t, producer.messages, 1)
	message := producer.messages[0]
	require.Equal(t, "metrics", message.Topic)
	encodedKey, err := message.Key.Encode()
	require.NoError(t, err)
	require.Equal(t, key, string(encodedKey))
	body, err := message.Value.Encode()
	require.NoError(t, err)
	var series []prompb.TimeSeries
	require.NoError(t, json.Unmarshal(body, &series))
	require.Equal(t, []prompb.TimeSeries{{
		Labels: []prompb.Label{{Name: "env", Value: "prod"}}, Samples: []prompb.Sample{{Value: 7, Timestamp: 2000}},
	}}, series)
}

func TestKafkaWriterWrite_RetriesFailedSend(t *testing.T) {
	key := cleanupWriteMetrics(t)
	producer := &round5KafkaProducer{failures: 1}
	writer := KafkaWriterType{Client: producer, RetryCount: 3, Opts: pconf.KafkaWriterOptions{Topic: "metrics"}}

	writer.Write(key, []prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "__name__", Value: "up"}},
		Samples: []prompb.Sample{{Value: 1}},
	}})

	require.Len(t, producer.messages, 2)
	first, err := producer.messages[0].Value.Encode()
	require.NoError(t, err)
	second, err := producer.messages[1].Value.Encode()
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, float64(1), testutil.ToFloat64(pstat.CounterWriteErrorTotal.WithLabelValues(key)))
}

func TestKafkaWriterWrite_SkipsEmptyAndDroppedBatches(t *testing.T) {
	key := cleanupWriteMetrics(t)
	producer := &round5KafkaProducer{}
	writer := KafkaWriterType{Client: producer, RetryCount: 1, Opts: pconf.KafkaWriterOptions{
		WriteRelabels: []*pconf.RelabelConfig{{Action: Drop, SourceLabels: model.LabelNames{"env"}, Regex: "prod"}},
	}}

	writer.Write(key, nil)
	writer.Write(key, []prompb.TimeSeries{{Labels: []prompb.Label{{Name: "env", Value: "prod"}}}})

	require.Empty(t, producer.messages)
	require.Zero(t, testutil.ToFloat64(pstat.CounterWriteTotal.WithLabelValues(key)))
}

func TestKafkaWriterWrite_EncodingErrorSkipsSend(t *testing.T) {
	key := cleanupWriteMetrics(t)
	producer := &round5KafkaProducer{}
	writer := KafkaWriterType{Client: producer, RetryCount: 1}

	writer.Write(key, []prompb.TimeSeries{{
		Labels:  []prompb.Label{{Name: "__name__", Value: "up"}},
		Samples: []prompb.Sample{{Value: math.Inf(1)}},
	}})

	require.Empty(t, producer.messages)
}

func TestInitKafkaSASL_AppliesEnabledCredentials(t *testing.T) {
	cfg := sarama.NewConfig()
	opt := pconf.KafkaWriterOptions{SASL: &pconf.SASLConfig{
		Enable: true, User: "writer", Password: "secret", Mechanism: "PLAIN",
		Version: 1, Handshake: true, AuthIdentity: "identity",
	}}

	initKafkaSASL(cfg, opt)

	require.True(t, cfg.Net.SASL.Enable)
	require.Equal(t, "writer", cfg.Net.SASL.User)
	require.Equal(t, "secret", cfg.Net.SASL.Password)
	require.Equal(t, sarama.SASLTypePlaintext, string(cfg.Net.SASL.Mechanism))
	require.Equal(t, int16(1), cfg.Net.SASL.Version)
	require.True(t, cfg.Net.SASL.Handshake)
	require.Equal(t, "identity", cfg.Net.SASL.AuthIdentity)
}
