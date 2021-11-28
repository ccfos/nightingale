package logx

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type Config struct {
	Dir        string
	Level      string
	Output     string
	KeepHours  uint
	RotateNum  int
	RotateSize uint64
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

		logger.SetLogging(c.Level, lb)
	}

	return func() {
		fmt.Println("logger exiting")
		logger.Close()
	}, nil
}
