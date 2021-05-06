package ping

import (
	"time"

	ping "github.com/sparrc/go-ping"
	"github.com/toolkits/pkg/logger"
)

type ipRes struct {
	IP   string
	Good bool
}

func FilterIP(ips []string) []string {
	workerNum := 100
	worker := make(chan struct{}, workerNum) // 控制 goroutine 并发数
	dataChan := make(chan *ipRes, 20000)
	done := make(chan struct{}, 1)
	goodIps := []string{}

	go func() {
		defer func() { done <- struct{}{} }()
		for d := range dataChan {
			if d.Good {
				goodIps = append(goodIps, d.IP)
			}
		}
	}()

	for _, ip := range ips {
		worker <- struct{}{}
		go fastPingRtt(ip, 300, worker, dataChan)
	}

	// 等待所有 goroutine 执行完成
	for i := 0; i < workerNum; i++ {
		worker <- struct{}{}
	}

	close(dataChan)
	<-done

	return goodIps
}

func fastPingRtt(ip string, timeout int, worker chan struct{}, dataChan chan *ipRes) {
	defer func() {
		<-worker
	}()
	res := &ipRes{
		IP:   ip,
		Good: goping(ip, timeout),
	}
	dataChan <- res
}

func goping(ip string, timeout int) bool {
	pinger, err := ping.NewPinger(ip)
	if err != nil {
		panic(err)
	}

	pinger.SetPrivileged(true)
	pinger.Count = 2
	pinger.Timeout = time.Duration(timeout) * time.Millisecond
	pinger.Interval = time.Duration(timeout) * time.Millisecond
	pinger.Run()                 // blocks until finished
	stats := pinger.Statistics() // get send/receive/rtt stats
	if stats.PacketsRecv > 0 {
		return true
	}

	logger.Debugf("%+v\n", stats)
	return false
}
