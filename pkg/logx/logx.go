package logx

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type Config struct {
	Dir             string
	Level           string
	Output          string
	KeepHours       uint
	RotateNum       int
	RotateSize      uint64
	OutputToOneFile bool
}

func Init(c Config) (func(), error) {
	logger.SetSeverity(c.Level)

	if c.Output == "stderr" {
		logger.LogToStderr()
	} else if c.Output == "file" {
		lb, err := logger.NewFileBackend(c.Dir)
		if err != nil {
			return nil, errors.WithMessage(err, "NewFileBackend failed")
		}

		if c.KeepHours != 0 {
			lb.SetRotateByHour(true)
			lb.SetKeepHours(c.KeepHours)
		} else if c.RotateNum != 0 {
			lb.Rotate(c.RotateNum, c.RotateSize*1024*1024)
		} else {
			return nil, errors.New("KeepHours and Rotatenum both are 0")
		}
		lb.OutputToOneFile(c.OutputToOneFile)

		logger.SetLogging(c.Level, lb)
	}

	return func() {
		fmt.Println("logger exiting")
		logger.Close()
	}, nil
}

// traceKey is the context key for storing traceId.
type traceKey struct{}

// NewTraceContext returns a new context carrying the given traceId.
func NewTraceContext(ctx context.Context, traceId string) context.Context {
	return context.WithValue(ctx, traceKey{}, traceId)
}

// GetTraceId extracts the traceId from ctx, or returns "" if absent.
func GetTraceId(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(traceKey{}).(string)
	return id
}

func prefix(ctx context.Context) string {
	id := GetTraceId(ctx)
	if id == "" {
		return ""
	}
	return "trace_id=" + id + " "
}

func Infof(ctx context.Context, format string, args ...interface{}) {
	logger.Infof(prefix(ctx)+format, args...)
}

func Errorf(ctx context.Context, format string, args ...interface{}) {
	logger.Errorf(prefix(ctx)+format, args...)
}

func Warningf(ctx context.Context, format string, args ...interface{}) {
	logger.Warningf(prefix(ctx)+format, args...)
}

func Debugf(ctx context.Context, format string, args ...interface{}) {
	logger.Debugf(prefix(ctx)+format, args...)
}
