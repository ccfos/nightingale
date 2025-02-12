package kafka

import (
	"fmt"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	AsyncProducer = "async"
	SyncProducer  = "sync"
)

var (
	KafkaProducerSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_producer_message_success_total",
			Help: "Total number of successful messages sent to Kafka.",
		},
		[]string{"producer_type"},
	)

	KafkaProducerError = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_producer_message_error_total",
			Help: "Total number of errors encountered while sending messages to Kafka.",
		},
		[]string{"producer_type"},
	)
)

func init() {
	prometheus.MustRegister(
		KafkaProducerSuccess,
		KafkaProducerError,
	)
}

type (
	Producer interface {
		Send(*sarama.ProducerMessage) error
		Close() error
	}

	AsyncProducerWrapper struct {
		asyncProducer sarama.AsyncProducer
		stop          chan struct{}
	}

	SyncProducerWrapper struct {
		syncProducer sarama.SyncProducer
		stop         chan struct{}
	}
)

func New(typ string, brokers []string, config *sarama.Config) (Producer, error) {
	stop := make(chan struct{})
	switch typ {
	case AsyncProducer:
		p, err := sarama.NewAsyncProducer(brokers, config)
		if err != nil {
			return nil, err
		}
		apw := &AsyncProducerWrapper{
			asyncProducer: p,
			stop:          stop,
		}
		go apw.errorWorker()
		go apw.successWorker()
		return apw, nil
	case SyncProducer:
		if !config.Producer.Return.Successes {
			config.Producer.Return.Successes = true
		}
		p, err := sarama.NewSyncProducer(brokers, config)
		return &SyncProducerWrapper{syncProducer: p}, err
	default:
		return nil, fmt.Errorf("unknown producer type: %s", typ)
	}
}

func (p *AsyncProducerWrapper) Send(msg *sarama.ProducerMessage) error {
	p.asyncProducer.Input() <- msg
	return nil
}

func (p *AsyncProducerWrapper) Close() error {
	close(p.stop)
	return p.asyncProducer.Close()
}

func (p *AsyncProducerWrapper) errorWorker() {
	for {
		select {
		case <-p.asyncProducer.Errors():
			KafkaProducerError.WithLabelValues(AsyncProducer).Inc()
		case <-p.stop:
			return
		}
	}
}

func (p *AsyncProducerWrapper) successWorker() {
	for {
		select {
		case <-p.asyncProducer.Successes():
			KafkaProducerSuccess.WithLabelValues(AsyncProducer).Inc()
		case <-p.stop:
			return
		}
	}
}

func (p *SyncProducerWrapper) Send(msg *sarama.ProducerMessage) error {
	_, _, err := p.syncProducer.SendMessage(msg)
	if err == nil {
		KafkaProducerSuccess.WithLabelValues(SyncProducer).Inc()
	} else {
		KafkaProducerError.WithLabelValues(SyncProducer).Inc()
	}
	return err
}

func (p *SyncProducerWrapper) Close() error {
	close(p.stop)
	return p.syncProducer.Close()
}
