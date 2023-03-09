package memsto

import (
	"os"

	"github.com/toolkits/pkg/logger"
)

// TODO 优化 exit 处理方式
func exit(code int) {
	logger.Close()
	os.Exit(code)
}
