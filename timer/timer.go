package timer

import (
	"os"

	"github.com/toolkits/pkg/logger"
)

func exit(code int) {
	logger.Close()
	os.Exit(code)
}
