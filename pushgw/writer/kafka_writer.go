package writer

import (
	"time"

	"github.com/IBM/sarama"
	"github.com/ccfos/nightingale/v6/pushgw/kafka"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"
)

type KafkaWriterType struct {
	Opts             pconf.KafkaWriterOptions
	ForceUseServerTS bool
	Client           kafka.Producer
	RetryCount       int
	RetryInterval    int64 // 单位秒
}

func (w KafkaWriterType) Write(key string, items []prompb.TimeSeries, headers ...map[string]string) {
	if len(items) == 0 {
		return
	}

	items = Relabel(items, w.Opts.WriteRelabels)
	if len(items) == 0 {
		return
	}

	start := time.Now()
	defer func() {
		ForwardDuration.WithLabelValues(key).Observe(time.Since(start).Seconds())
	}()

	data, err := beforeWrite(key, items, w.ForceUseServerTS, "json")
	if err != nil {
		logger.Warningf("marshal prom data to proto got error: %v, data: %+v", err, items)
		return
	}

	for i := 0; i < w.RetryCount; i++ {
		err := w.Client.Send(&sarama.ProducerMessage{Topic: w.Opts.Topic,
			Key: sarama.StringEncoder(key), Value: sarama.ByteEncoder(data)})
		if err == nil {
			break
		}

		CounterWirteErrorTotal.WithLabelValues(key).Add(float64(len(items)))
		logger.Warningf("send to kafka got error: %v in %d times, broker: %v, topic: %s",
			err, i, w.Opts.Brokers, w.Opts.Topic)

		if i == 0 {
			logger.Warning("example timeseries:", items[0].String())
		}

		time.Sleep(time.Duration(w.RetryInterval) * time.Second)
	}
}
