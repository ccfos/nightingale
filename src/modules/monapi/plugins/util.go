package plugins

import (
	"fmt"

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
