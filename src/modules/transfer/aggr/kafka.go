package aggr

import (
	"time"

	"github.com/Shopify/sarama"
	"github.com/toolkits/pkg/logger"
)

var KafkaProducer producer

type producer struct {
	asyncProducer sarama.AsyncProducer
}

func InitKakfa(aggr AggrSection) {
	var err error
	kafkaAddrs := aggr.KafkaAddrs

	cfg := sarama.NewConfig()
	cfg.ClientID = "transfer"
	cfg.ChannelBufferSize = 256

	cfg.Net.DialTimeout = 1000 * time.Millisecond
	cfg.Net.ReadTimeout = 2 * time.Second
	cfg.Net.WriteTimeout = 2 * time.Second
	cfg.Net.KeepAlive = 10 * time.Minute

	cfg.Producer.RequiredAcks = sarama.WaitForLocal
	cfg.Producer.Compression = sarama.CompressionGZIP

	// 2M是建议值, 超过2M失败的概率会增大
	cfg.Producer.MaxMessageBytes = 2000000

	cfg.Producer.Partitioner = sarama.NewRoundRobinPartitioner

	cfg.Producer.Flush.MaxMessages = 256
	cfg.Producer.Flush.Frequency = 1 * time.Second

	KafkaProducer.asyncProducer, err = sarama.NewAsyncProducer(kafkaAddrs, cfg)
	if err != nil {
		logger.Errorf("create kafka producer err: %v\n", err)
		return //重启的时候，注意观察报错，防止 Kafka 启动失败没有发现
	}

	go KafkaProducer.handleKafkaError()
}

func (this producer) producePoints(msg []byte) {
	//tStart := time.Now()

	this.asyncProducer.Input() <- &sarama.ProducerMessage{Topic: AggrConfig.KafkaAggrInTopic, Key: nil, Value: sarama.ByteEncoder(msg)}

}

func (this producer) handleKafkaError() {
	for {
		err := <-this.asyncProducer.Errors()
		logger.Errorf("failed to produce to kafka:%v\n", err.Err)
	}
}

func (this producer) Close() {
	if err := this.asyncProducer.Close(); err != nil {
		logger.Errorf("failed to close Kafka producer cleanly: %v\n", err)
	}
}
