package logx

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	tklog "github.com/toolkits/pkg/logger"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

var (
	InfoStr      = "%s\n[info] "
	WarnStr      = "%s\n[warn] "
	ErrStr       = "%s\n[error] "
	TraceStr     = "%s\n[%.3fms] [rows:%v] %s"
	TraceWarnStr = "%s %s\n[%.3fms] [rows:%v] %s"
	TraceErrStr  = "%s %s\n[%.3fms] [rows:%v] %s"
)

type GORMLogger struct {
	*tklog.Logger
	LogLevel                  logger.LogLevel
	SlowThreshold             time.Duration
	IgnoreRecordNotFoundError bool
	InfoStr                   string
	WarnStr                   string
	ErrStr                    string
	TraceStr                  string
	TraceWarnStr              string
	TraceErrStr               string
}

func (gl *GORMLogger) LogMode(level logger.LogLevel) logger.Interface {
	newlogger := *gl
	newlogger.LogLevel = level
	return &newlogger
}

func (gl *GORMLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if gl.LogLevel >= logger.Info {
		gl.Infof(gl.InfoStr+msg, append([]interface{}{utils.FileWithLineNum()}, data...)...)
	}
}

func (gl *GORMLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if gl.LogLevel >= logger.Warn {
		gl.Warningf(gl.WarnStr+msg, append([]interface{}{utils.FileWithLineNum()}, data...)...)
	}
}

func (gl *GORMLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if gl.LogLevel >= logger.Warn {
		gl.Errorf(gl.ErrStr+msg, append([]interface{}{utils.FileWithLineNum()}, data...)...)
	}
}

// Trace print sql message
func (gl *GORMLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if gl.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && gl.LogLevel >= logger.Error && (!errors.Is(err, logger.ErrRecordNotFound) || !gl.IgnoreRecordNotFoundError):
		sql, rows := fc()
		if rows == -1 {
			gl.Errorf(gl.TraceErrStr, utils.FileWithLineNum(), err, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			gl.Errorf(gl.TraceErrStr, utils.FileWithLineNum(), err, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case elapsed > gl.SlowThreshold && gl.SlowThreshold != 0 && gl.LogLevel >= logger.Warn:
		sql, rows := fc()
		slowLog := fmt.Sprintf("SLOW SQL >= %v", gl.SlowThreshold)
		if rows == -1 {
			gl.Warningf(gl.TraceWarnStr, utils.FileWithLineNum(), slowLog, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			gl.Warningf(gl.TraceWarnStr, utils.FileWithLineNum(), slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case gl.LogLevel == logger.Info:
		sql, rows := fc()
		if rows == -1 {
			gl.Infof(gl.TraceStr, utils.FileWithLineNum(), float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			gl.Infof(gl.TraceStr, utils.FileWithLineNum(), float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	}
}
