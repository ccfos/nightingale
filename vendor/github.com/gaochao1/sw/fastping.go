package sw

import (
	"github.com/freedomkk-qfeng/go-fastping"
	"net"
	"time"
)

func fastPingRtt(ip string, timeout int) (float64, error) {
	var rt float64
	rt = -1
	p := fastping.NewPinger()
	ra, err := net.ResolveIPAddr("ip4:icmp", ip)
	if err != nil {
		return -1, err
	}
	p.AddIPAddr(ra)
	p.OnRecv = func(addr *net.IPAddr, rtt time.Duration) {
		rt = float64(rtt.Nanoseconds()) / 1000000.0
		//fmt.Printf("IP Addr: %s receive, RTT: %v\n", addr.String(), rtt)
	}
	p.OnIdle = func() {
		//fmt.Println("finish")
	}
	p.MaxRTT = time.Millisecond * time.Duration(timeout)
	err = p.Run()
	if err != nil {
		return -1, err
	}

	return rt, err
}
