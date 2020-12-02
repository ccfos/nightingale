package rabbitmq

import (
	"time"

	"github.com/streadway/amqp"
	"github.com/toolkits/pkg/logger"
)

// Consume 消费消息
func Consume(url, queueName string) {
	for {
		select {
		case <-exit:
			return
		default:
			if err := ping(); err != nil {
				logger.Error("rabbitmq conn failed")
				conn, err = amqp.Dial(url)
				if err != nil {
					conn = nil
					logger.Error(err)
					time.Sleep(500 * time.Millisecond)
					continue
				}
			}

			sleep := consume(queueName)
			if sleep {
				time.Sleep(300 * time.Millisecond)
			}
		}
	}
}

// 如果操作MQ出现问题，或者没有load到数据，就sleep一下
func consume(queueName string) bool {
	if conn == nil {
		return true
	}

	ch, err := conn.Channel()
	if err != nil {
		logger.Error(err)
		return true
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
		return true
	}

	err = ch.Qos(
		80,    // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		logger.Error(err)
		return true
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
		return true
	}

	size := 0

	isSleep := false
	for d := range msgs {
		size++
		logger.Infof("rabbitmq consume message: %s", d.Body)

		if handleMessage(d.Body) {
			d.Ack(true)
		} else {
			// 底层代码认为不应该ack，说明处理的过程出现问题，可能是DB有问题之类的，sleep一下
			isSleep = true
			continue
		}
	}

	if isSleep {
		return true
	}

	if size == 0 {
		// MQ里没有消息，就sleep一下，否则上层代码一直在死循环空转，浪费算力
		return true
	}

	return false
}
