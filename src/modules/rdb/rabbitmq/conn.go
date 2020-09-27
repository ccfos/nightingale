package rabbitmq

import (
	"fmt"
	"log"
	"time"

	"github.com/streadway/amqp"
	"github.com/toolkits/pkg/logger"
)

var (
	conn *amqp.Connection
	exit = make(chan bool)
)

func Init(url string) {
	var err error
	conn, err = amqp.Dial(url)
	if err != nil {
		log.Fatal(err)
	}

	go healthCheck(url)
}

func healthCheck(url string) {
	ticker := time.NewTicker(40 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			err := ping()
			if err != nil {
				defer func() {
					if err := recover(); err != nil {
						conn = nil
						logger.Error(err)
					}
				}()
				conn, err = amqp.Dial(url)
				if err != nil {
					logger.Error(err)
				}
			}
		case <-exit:
			return
		}
	}
}

// ping 测试连接是否正常
func ping() (err error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error(err)
		}
	}()

	if conn == nil {
		return fmt.Errorf("conn is nil")
	}

	ch, err := conn.Channel()
	if err != nil {
		logger.Error(err)
		return err
	}

	defer ch.Close()

	err = ch.ExchangeDeclare("ping.ping", "topic", false, true, false, true, nil)
	if err != nil {
		logger.Error(err)
		return err
	}

	msgContent := "ping.ping"
	err = ch.Publish("ping.ping", "ping.ping", false, false, amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte(msgContent),
	})
	if err != nil {
		logger.Error(err)
		return err
	}
	err = ch.ExchangeDelete("ping.ping", false, false)
	if err != nil {
		logger.Error(err)
	}

	return err
}

func Shutdown() {
	conn.Close()
	exit <- true
}
