package pconf

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPreCheck_AppliesOperationalDefaults(t *testing.T) {
	config := Pushgw{}

	config.PreCheck()

	require.Equal(t, 3, config.UpdateTargetRetryCount)
	require.Equal(t, int64(500), config.UpdateTargetRetryIntervalMills)
	require.Equal(t, int64(3000), config.UpdateTargetTimeoutMills)
	require.Equal(t, 20, config.UpdateTargetBatchSize)
	require.Equal(t, 10, config.UpdateTargetByUrlConcurrency)
	require.Equal(t, "busigroup", config.BusiGroupLabelKey)
	require.Equal(t, 5000, config.WriteConcurrency)
	require.Equal(t, 1500, config.IdentStatsThreshold)
	require.Equal(t, 5000000, config.IdentDropThreshold)
	require.Equal(t, 1000, config.ProxyInflightMax)
	require.Equal(t, int64(32*1024*1024), config.ProxyMaxBodyBytes)
	wantQueues := runtime.NumCPU()
	if wantQueues <= 1 {
		wantQueues = 128
	}
	require.Equal(t, WriterGlobalOpt{
		QueueMaxSize: 1000000, QueuePopSize: 1000, QueueNumber: wantQueues, QueueWaterMark: 0.1,
		AllQueueMaxSize: int64(wantQueues * 100000), AllQueueMaxSizeInterval: 200,
		RetryCount: 1000, RetryInterval: 1, OverLimitStatusCode: 499,
	}, config.WriterOpt)
}

func TestPreCheck_PreservesExplicitLimitsAndComputesQueueBudget(t *testing.T) {
	config := Pushgw{
		UpdateTargetRetryCount: 4, UpdateTargetRetryIntervalMills: 12, UpdateTargetTimeoutMills: 34,
		UpdateTargetBatchSize: 6, UpdateTargetByUrlConcurrency: 7, BusiGroupLabelKey: "team",
		WriteConcurrency: 8, IdentStatsThreshold: 9, IdentDropThreshold: 10,
		ProxyInflightMax: 11, ProxyMaxBodyBytes: 12,
		WriterOpt: WriterGlobalOpt{
			QueueMaxSize: 100, QueuePopSize: 20, QueueNumber: 3, QueueWaterMark: 0.5,
			AllQueueMaxSizeInterval: 30, RetryCount: 4, RetryInterval: 5, OverLimitStatusCode: 429,
		},
	}
	expected := config
	expected.WriterOpt.AllQueueMaxSize = 150

	config.PreCheck()

	require.Equal(t, expected, config)
}

func TestPreCheck_InitializesWriterTimeoutsAndDefaultRelabel(t *testing.T) {
	relabel := &RelabelConfig{}
	config := Pushgw{Writers: []WriterOptions{{WriteRelabels: []*RelabelConfig{relabel}}}}

	config.PreCheck()

	writer := config.Writers[0]
	t.Cleanup(writer.HTTPTransport.CloseIdleConnections)
	require.Equal(t, int64(10000), writer.Timeout)
	require.Equal(t, int64(3000), writer.DialTimeout)
	require.Equal(t, int64(30000), writer.TLSHandshakeTimeout)
	require.Equal(t, 10*time.Second, writer.HTTPTransport.ResponseHeaderTimeout)
	require.Equal(t, 30*time.Second, writer.HTTPTransport.TLSHandshakeTimeout)
	require.Equal(t, "(.*)", relabel.Regex)
	require.Equal(t, ";", relabel.Separator)
	require.Equal(t, "replace", relabel.Action)
	require.Equal(t, "$1", relabel.Replacement)
	require.True(t, relabel.RegexCompiled.MatchString("anything"))
}

func TestPreCheck_CompilesAnchoredRelabelAndPreservesTransportSettings(t *testing.T) {
	relabel := &RelabelConfig{Regex: "prod|stage", Separator: ",", Action: "keep", Replacement: "fixed"}
	config := Pushgw{Writers: []WriterOptions{{
		Timeout: 10, DialTimeout: 20, TLSHandshakeTimeout: 30, ExpectContinueTimeout: 40,
		IdleConnTimeout: 50, MaxConnsPerHost: 6, MaxIdleConns: 7, MaxIdleConnsPerHost: 8,
		WriteRelabels: []*RelabelConfig{relabel},
	}}}

	config.PreCheck()

	writer := config.Writers[0]
	transport := writer.HTTPTransport
	t.Cleanup(transport.CloseIdleConnections)
	require.Equal(t, int64(20), writer.DialTimeout)
	require.Equal(t, 10*time.Millisecond, transport.ResponseHeaderTimeout)
	require.Equal(t, 30*time.Millisecond, transport.TLSHandshakeTimeout)
	require.Equal(t, 40*time.Millisecond, transport.ExpectContinueTimeout)
	require.Equal(t, 50*time.Millisecond, transport.IdleConnTimeout)
	require.Equal(t, 6, transport.MaxConnsPerHost)
	require.Equal(t, 7, transport.MaxIdleConns)
	require.Equal(t, 8, transport.MaxIdleConnsPerHost)
	require.True(t, relabel.RegexCompiled.MatchString("prod"))
	require.False(t, relabel.RegexCompiled.MatchString("production"))
	require.Equal(t, ",", relabel.Separator)
	require.Equal(t, "keep", relabel.Action)
	require.Equal(t, "fixed", relabel.Replacement)
}
