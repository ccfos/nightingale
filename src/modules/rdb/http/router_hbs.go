package http

import (
	"time"

	"github.com/didi/nightingale/src/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

func heartBeat(c *gin.Context) {
	var rev models.Instance
	errors.Dangerous(c.ShouldBind(&rev))
	instance, err := models.GetInstanceBy(rev.Module, rev.Identity, rev.RPCPort, rev.HTTPPort)
	errors.Dangerous(err)

	now := time.Now().Unix()
	if instance == nil {
		instance = &models.Instance{
			Identity: rev.Identity,
			Module:   rev.Module,
			RPCPort:  rev.RPCPort,
			HTTPPort: rev.HTTPPort,
			Region:   rev.Region,
			TS:       now,
		}
		errors.Dangerous(instance.Add())
	} else {
		instance.TS = now
		instance.HTTPPort = rev.HTTPPort
		instance.Region = rev.Region
		errors.Dangerous(instance.Update())
	}

	renderData(c, "ok", nil)
}

func instanceGets(c *gin.Context) {
	mod := queryStr(c, "mod")
	alive := queryInt(c, "alive", 0)

	instances, err := models.GetAllInstances(mod, alive)

	renderData(c, instances, err)
}

type instanceDelRev struct {
	Ids []int64 `json:"ids"`
}

func instanceDel(c *gin.Context) {
	username := loginUsername(c)
	if username != "root" {
		errors.Bomb("permission deny")
	}

	var rev instanceDelRev
	errors.Dangerous(c.ShouldBind(&rev))
	for _, id := range rev.Ids {
		errors.Dangerous(models.DelById(id))
		logger.Infof("[index] %s delete %+v", username, id)
	}
	renderData(c, "ok", nil)
}
