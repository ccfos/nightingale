package rabbitmq

import (
	"time"

	"github.com/toolkits/pkg/logger"
)

// Consume 消费消息
func Consume(queueName string) {
	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			if mqErr := consume(queueName); mqErr != nil {
				conn = nil
			}
		case <-exit:
			return
		}
	}
}

// consume 如果操作MQ出现问题,连接置为空
func consume(queueName string) error {
	defer func() {
		if err := recover(); err != nil {
			logger.Error(err)
		}
	}()

	if conn == nil {
		return nil
	}

	ch, err := conn.Channel()
	if err != nil {
		logger.Error(err)
		return err
	}

	defer ch.Close()

	q, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		logger.Error(err)
		return err
	}

	err = ch.Qos(
		80,    // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		logger.Error(err)
		return err
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		logger.Error(err)
		return err
	}

	for d := range msgs {
		logger.Infof("rabbitmq consume message: %s", d.Body)

		if handleMessage(d.Body) {
			d.Ack(true)
		}
	}

	return nil
}
