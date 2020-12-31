package ports

import (
	"fmt"
	"net"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/agent/config"
	"github.com/didi/nightingale/src/modules/agent/core"
	"github.com/didi/nightingale/src/modules/agent/report"
)

type PortScheduler struct {
	Ticker *time.Ticker
	Port   *models.PortCollect
	Quit   chan struct{}
}

func NewPortScheduler(p *models.PortCollect) *PortScheduler {
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

func PortCollect(p *models.PortCollect) {
	value := 0
	if isListen(p.Port, 1, "127.0.0.1") {
		value = 1
	} else if isListen(p.Port, 1, report.IP) {
		value = 1
	} else if isListen(p.Port, 1, "::1") {
		value = 1
	}

	item := core.GaugeValue("proc.port.listen", value, p.Tags)
	item.Step = int64(p.Step)
	item.Timestamp = time.Now().Unix()
	item.Endpoint = config.Endpoint
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
