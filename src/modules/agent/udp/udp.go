package udp

import (
	"fmt"
	"log"
	"net"

	"github.com/didi/nightingale/src/modules/agent/config"
)

var (
	udpConn *net.UDPConn = nil
)

func Start() {
	if !config.Config.Udp.Enable {
		log.Println("udp server disabled")
		return
	}

	address, _ := net.ResolveUDPAddr("udp4", config.Config.Udp.Listen)
	conn, err := net.ListenUDP("udp4", address)
	if err != nil {
		errsmg := fmt.Sprintf("listen udp error, [addr: %s][error: %s]", config.Config.Udp.Listen, err.Error())
		log.Printf(errsmg)
		panic(errsmg)
	}
	log.Println("udp start, listening on ", config.Config.Udp.Listen)

	// 保存 udp服务链接
	udpConn = conn

	// 开启 udp数据包处理进程
	go handleUdpPackets()
}

func stop() error {
	if udpConn != nil {
		udpConn.Close()
	}
	return nil
}
