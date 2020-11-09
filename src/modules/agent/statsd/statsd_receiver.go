package statsd

import (
	"strings"

	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

type StatsdReceiver struct{}

func (self StatsdReceiver) HandlePacket(packet string) {
	lines := strings.SplitN(packet, "\n", 3)
	if len(lines) != 3 {
		logger.Warningf("invalid packet, [error: missing args][packet: %s]", packet)
		return
	}

	value := lines[0]
	//
	argLines, aggrs, err := Func{}.FormatArgLines(lines[2], lines[1])
	if err != nil {
		if err.Error() == "ignore" {
			return
		}
		logger.Warningf("invalid packet, [error: bad tags or aggr][msg: %s][packet: %s]", err.Error(), packet)
		return
	}
	metric, err := Func{}.FormatMetricLine(lines[1], aggrs) // metric = $ns/$metric_name
	if err != nil {
		logger.Warningf("invalid packet, [error: bad metric line][msg: %s][packet %s]", err.Error(), packet)
		return
	}

	stats.Counter.Set("metric.recv.packet", 1)

	err = StatsdState{}.GetState().Collect(value, metric, argLines)
	if err != nil {
		logger.Warningf("invalid packet, [error: collect packet error][msg: %s][packet: %s]", err.Error(), packet)
		return
	}
}
