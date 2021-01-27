package plugins

import (
	"fmt"
	"testing"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/prober/manager"
	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
)

var defaultLogger = Logger{}

func GetLogger() *Logger {
	return &defaultLogger
}

// telegraf.Logger
type Logger struct{}

func (l *Logger) Errorf(format string, args ...interface{}) {
	logger.LogDepth(logger.ERROR, 1, format, args...)
}
func (l *Logger) Error(args ...interface{}) {
	logger.LogDepth(logger.ERROR, 1, fmt.Sprint(args...))
}
func (l *Logger) Debugf(format string, args ...interface{}) {
	logger.LogDepth(logger.DEBUG, 1, format, args...)
}
func (l *Logger) Debug(args ...interface{}) {
	logger.LogDepth(logger.DEBUG, 1, fmt.Sprint(args...))
}
func (l *Logger) Warnf(format string, args ...interface{}) {
	logger.LogDepth(logger.WARNING, 1, format, args...)
}
func (l *Logger) Warn(args ...interface{}) {
	logger.LogDepth(logger.WARNING, 1, fmt.Sprint(args...))
}
func (l *Logger) Infof(format string, args ...interface{}) {
	logger.LogDepth(logger.INFO, 1, format, args...)
}
func (l *Logger) Info(args ...interface{}) {
	logger.LogDepth(logger.INFO, 1, fmt.Sprint(args...))
}

type telegrafPlugin interface {
	TelegrafInput() (telegraf.Input, error)
}

func PluginTest(t *testing.T, plugin telegrafPlugin) {
	metrics := []*dataobj.MetricValue{}

	input, err := plugin.TelegrafInput()
	if err != nil {
		t.Error(err)
	}

	acc, err := manager.NewAccumulator(manager.AccumulatorOptions{Name: "github-test", Metrics: &metrics})
	if err != nil {
		t.Error(err)
	}

	if err = input.Gather(acc); err != nil {
		t.Error(err)
	}

	for k, v := range metrics {
		t.Logf("%d %s %s %f", k, v.CounterType, v.PK(), v.Value)
	}
}
