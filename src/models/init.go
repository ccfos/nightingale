package models

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/toolkits/pkg/cache"
)

func init() {
	cache.InitMemoryCache(time.Hour)
}

func _e(format string, a ...interface{}) error {
	return fmt.Errorf(i18n.Sprintf(format, a...))
}

func _s(format string, a ...interface{}) string {
	return i18n.Sprintf(format, a...)
}
