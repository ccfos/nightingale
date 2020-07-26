package ports

import (
	"fmt"
	"net"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/collector/core"
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

func (p *PortScheduler) Schedule() {
	go func() {
		for {
			select {
			case <-p.Ticker.C:
				PortCollect(p.Port)
			case <-p.Quit:
				p.Ticker.Stop()
				return
			}
		}
	}()
}

func (p *PortScheduler) Stop() {
	close(p.Quit)
}

func PortCollect(p *model.PortCollect) {
	value := 0
	if isListening(p.Port) {
		value = 1
	}

	item := core.GaugeValue("proc.port.listen", value, p.Tags)
	item.Step = int64(p.Step)
	item.Timestamp = time.Now().Unix()
	item.Endpoint = identity.Identity
	core.Push([]*dataobj.MetricValue{item})
}

func isListening(port int) bool {
	tcpAddress, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf(":%v", port))
	if err != nil {
		logger.Errorf("net.ResolveTCPAddr(tcp4, :%v) fail: %v", port, err)
		return false
	}

	listener, err := net.ListenTCP("tcp", tcpAddress)
	if err != nil {
		logger.Debugf("cannot listen :%v(%v), so we think :%v is already listening", port, err, port)
		return true
	}
	listener.Close()

	return false
}

func isListen(port, timeout int, ip string) bool {
	var conn net.Conn
	var err error
	addr := fmt.Sprintf("%s:%d", ip, port)
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
