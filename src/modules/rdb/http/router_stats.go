package http

import (
	"github.com/didi/nightingale/src/models"
	"github.com/gin-gonic/gin"
)

type rdbStats struct {
	Login *models.Stats
}

var (
	stats *rdbStats
)

func initStats() {
	stats = &rdbStats{
		Login: models.MustNewStats("login"),
	}
}

func counterGet(c *gin.Context) {
	renderData(c, map[string]int64{
		"login": stats.Login.Get(),
	}, nil)
}
