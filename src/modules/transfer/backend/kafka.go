package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/Shopify/sarama"
	"github.com/toolkits/pkg/logger"
)

type KafkaSection struct {
	Enabled      bool   `yaml:"enabled"`
	Name         string `yaml:"name"`
	Topic        string `yaml:"topic"`
	BrokersPeers string `yaml:"brokersPeers"`
	ConnTimeout  int    `yaml:"connTimeout"`
	CallTimeout  int    `yaml:"callTimeout"`
	MaxRetry     int    `yaml:"maxRetry"`
	KeepAlive    int64  `yaml:"keepAlive"`
	SaslUser     string `yaml:"saslUser"`
	SaslPasswd   string `yaml:"saslPasswd"`
}
type KafkaPushEndpoint struct {
	// config
	Section KafkaSection

	// 发送缓存队列 node -> queue_of_data
	KafkaQueue chan KafkaData
}

func (kafka *KafkaPushEndpoint) Init() {

	// init queue
	kafka.KafkaQueue = make(chan KafkaData, 10)

	// start task
	go kafka.send2KafkaTask()
}

func (kafka *KafkaPushEndpoint) Push2Queue(items []*dataobj.MetricValue) {
	for _, item := range items {
		kafka.KafkaQueue <- kafka.convert2KafkaItem(item)
	}
}

func (kafka *KafkaPushEndpoint) convert2KafkaItem(d *dataobj.MetricValue) KafkaData {
	m := make(KafkaData)
	m["metric"] = d.Metric
	m["value"] = d.Value
	m["timestamp"] = d.Timestamp
	m["value"] = d.Value
	m["step"] = d.Step
	m["endpoint"] = d.Endpoint
	m["tags"] = d.Tags
	return m
}

func (kafka *KafkaPushEndpoint) send2KafkaTask() {
	kf, err := NewKfClient(kafka.Section)
	if err != nil {
		logger.Errorf("init kafka client fail: %v", err)
		return
	}
	defer kf.Close()
	for {
		kafkaItem := <-kafka.KafkaQueue
		stats.Counter.Set("points.out.kafka", 1)
		err = kf.Send(kafkaItem)
		if err != nil {
			stats.Counter.Set("points.out.kafka.err", 1)
			logger.Errorf("send %v to kafka %s fail: %v", kafkaItem, kafka.Section.BrokersPeers, err)
		}
	}
}

type KafkaData map[string]interface{}
type KfClient struct {
	producer     sarama.AsyncProducer
	cfg          *sarama.Config
	Topic        string
	BrokersPeers []string
	ticker       *time.Ticker
}

func NewKfClient(c KafkaSection) (kafkaSender *KfClient, err error) {
	topic := c.Topic
	if len(topic) == 0 {
		err = errors.New("topic is nil")
		return
	}
	brokers := strings.Split(c.BrokersPeers, ",")
	if len(brokers) == 0 {
		err = errors.New("brokers is nil")
		return
	}
	hostName, _ := os.Hostname()

	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.Return.Errors = true
	if len(hostName) > 0 {
		cfg.ClientID = hostName
	}
	cfg.Producer.Partitioner = func(topic string) sarama.Partitioner { return sarama.NewRoundRobinPartitioner(topic) }
	if len(c.SaslUser) > 0 && len(c.SaslPasswd) > 0 {
		cfg.Net.SASL.Enable = true
		cfg.Net.SASL.User = c.SaslUser
		cfg.Net.SASL.Password = c.SaslPasswd
	}
	if c.MaxRetry > 0 {
		cfg.Producer.Retry.Max = c.MaxRetry
	}

	cfg.Net.DialTimeout = time.Duration(c.ConnTimeout) * time.Millisecond

	if c.KeepAlive > 0 {
		cfg.Net.KeepAlive = time.Duration(c.KeepAlive) * time.Millisecond
	}
	producer, err := sarama.NewAsyncProducer(brokers, cfg)
	if err != nil {
		return
	}
	kafkaSender = newSender(brokers, topic, cfg, producer, c.CallTimeout)
	return
}
func newSender(brokers []string, topic string, cfg *sarama.Config, producer sarama.AsyncProducer,
	callTimeout int) (kf *KfClient) {
	kf = &KfClient{
		producer:     producer,
		Topic:        topic,
		BrokersPeers: brokers,
		ticker:       time.NewTicker(time.Millisecond * time.Duration(callTimeout)),
	}
	go kf.readMessageToErrorChan()
	return
}

func (kf *KfClient) readMessageToErrorChan() {
	var producer = kf.producer
	for {
		select {
		case <-producer.Successes():
		case errMsg := <-producer.Errors():
			msg, _ := errMsg.Msg.Value.Encode()
			logger.Errorf("ReadMessageToErrorChan err:%v %v", errMsg.Error(), string(msg))
		}
	}
}
func (kf *KfClient) Send(data KafkaData) error {
	var producer = kf.producer
	message, err := kf.getEventMessage(data)
	if err != nil {
		logger.Errorf("Dropping event: %v", err)
		return err
	}
	select {
	case producer.Input() <- message:
	case <-kf.ticker.C:
		return fmt.Errorf("send kafka failed:%v[%v]", kf.Topic, kf.BrokersPeers)
	}

	return nil
}

func (kf *KfClient) Close() error {
	logger.Infof("kafka sender(%s) was closed", kf.Topic, kf.BrokersPeers)
	_ = kf.producer.Close()
	kf.producer = nil
	return nil
}

func (kf *KfClient) getEventMessage(event map[string]interface{}) (pm *sarama.ProducerMessage, err error) {
	value, err := json.Marshal(event)
	if err != nil {
		return
	}
	pm = &sarama.ProducerMessage{
		Topic: kf.Topic,
		Value: sarama.StringEncoder(string(value)),
	}
	return
}
