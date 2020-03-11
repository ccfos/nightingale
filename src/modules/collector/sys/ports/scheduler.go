package ports

import (
	"fmt"
	"net"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/collector/sys/funcs"
	"github.com/didi/nightingale/src/toolkits/identity"
)

type PortScheduler struct {
	Ticker *time.Ticker
	Port   *model.PortCollect
	Quit   chan struct{}
}

func NewPortScheduler(p *model.PortCollect) *PortScheduler {
	scheduler := PortScheduler{Port: p}
	scheduler.Ticker = time.NewTicker(time.Duration(p.Step) * time.Second)
	scheduler.Quit = make(chan struct{})
	return &scheduler
}

func (this *PortScheduler) Schedule() {
	go func() {
		for {
			select {
			case <-this.Ticker.C:
				PortCollect(this.Port)
			case <-this.Quit:
				this.Ticker.Stop()
				return
			}
		}
	}()
}

func (this *PortScheduler) Stop() {
	close(this.Quit)
}

func PortCollect(p *model.PortCollect) {
	value := 0
	if isListening(p.Port, p.Timeout) {
		value = 1
	}

	item := funcs.GaugeValue("proc.port.listen", value, p.Tags)
	item.Step = int64(p.Step)
	item.Timestamp = time.Now().Unix()
	item.Endpoint = identity.Identity
	funcs.Push([]*dataobj.MetricValue{item})
}

func isListening(port int, timeout int) bool {
	var conn net.Conn
	var err error
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if timeout <= 0 {
		// default timeout 3 second
		timeout = 3
	}
	conn, err = net.DialTimeout("tcp", addr, time.Duration(timeout)*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
