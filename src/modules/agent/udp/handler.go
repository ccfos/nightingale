package udp

import (
	"sync"

	"github.com/didi/nightingale/src/modules/agent/statsd"
	"github.com/didi/nightingale/src/toolkits/exit"

	"github.com/toolkits/pkg/logger"
)

var ByteSlicePool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 4096, 4096)
	}}

func handleUdpPackets() {
	defer func() {
		if err := recover(); err != nil {
			stack := exit.Stack(3)
			logger.Warningf("udp handler exit unexpected, [error: %v],[stack: %s]", err, stack)
			panic(err) // udp异常, 为保证metrics功能完备性, 快速panic
		}
		// 停止udp服务
		stop()
	}()

	message := ByteSlicePool.Get().([]byte)
	for !statsd.IsExited() {
		n, _, err := udpConn.ReadFrom(message)
		if err != nil {
			logger.Warningf("read from udp error, [error: %s]", err.Error())
			continue
		}
		packet := string(message[0:n])
		ByteSlicePool.Put(message)

		logger.Debugf("recv packet: %v\n", packet)
		statsd.StatsdReceiver{}.HandlePacket(packet)
	}
}
