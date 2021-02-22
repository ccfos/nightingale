package plugins

import (
	"fmt"
	"reflect"
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

func SetValue(in interface{}, value interface{}, fields ...string) error {
	rv := reflect.Indirect(reflect.ValueOf(in))

	for _, field := range fields {
		if !rv.IsValid() {
			return fmt.Errorf("invalid argument")
		}
		if rv.Kind() != reflect.Struct {
			return fmt.Errorf("invalid argument, must be a struct")
		}
		rv = reflect.Indirect(rv.FieldByName(field))
	}

	if !rv.IsValid() || !rv.CanSet() {
		return fmt.Errorf("invalid argument IsValid %v CanSet %v", rv.IsValid(), rv.CanSet())
	}
	rv.Set(reflect.Indirect(reflect.ValueOf(value)))
	return nil
}

type telegrafPlugin interface {
	TelegrafInput() (telegraf.Input, error)
}

func PluginTest(t *testing.T, plugin telegrafPlugin) telegraf.Input {
	input, err := plugin.TelegrafInput()
	if err != nil {
		t.Error(err)
	}

	PluginInputTest(t, input)

	return input
}

func PluginInputTest(t *testing.T, input telegraf.Input) {
	metrics := []*dataobj.MetricValue{}

	acc, err := manager.NewAccumulator(manager.AccumulatorOptions{Name: "plugin-test", Metrics: &metrics})
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
