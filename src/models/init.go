package models

import (
	"time"

	"github.com/toolkits/pkg/cache"
)

func init() {
	cache.InitMemoryCache(time.Hour)
}
