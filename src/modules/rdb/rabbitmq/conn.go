package rabbitmq

import (
	"log"

	"github.com/streadway/amqp"
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
}

func Shutdown() {
	conn.Close()
	exit <- true
}
