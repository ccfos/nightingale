package loggeri

import (
	"fmt"
	"os"

	"github.com/toolkits/pkg/logger"
)

type Config struct {
	Dir        string `yaml:"dir"`
	Level      string `yaml:"level"`
	KeepHours  uint   `yaml:"keepHours"`
	Rotatenum  int    `yaml:"rotatenum"`
	Rotatesize uint64 `yaml:"rotatesize"`
}

// InitLogger init logger toolkit
func Init(c Config) {
	lb, err := logger.NewFileBackend(c.Dir)
	if err != nil {
		fmt.Println("cannot init logger:", err)
		os.Exit(1)
	}

	//设置了以小时切换文件，优先使用小时切割文件
	if c.KeepHours != 0 {
		lb.SetRotateByHour(true)
		lb.SetKeepHours(c.KeepHours)
	} else if c.Rotatenum != 0 {
		lb.Rotate(c.Rotatenum, c.Rotatesize*1024*1024)
	} else {
		fmt.Println("cannot init logger: KeepHours and Rotatenum is 0")
		os.Exit(2)
	}

	logger.SetLogging(c.Level, lb)
}
