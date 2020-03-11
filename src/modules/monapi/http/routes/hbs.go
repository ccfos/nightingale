package routes

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/model"
)

func heartBeat(c *gin.Context) {
	var rev model.Instance
	errors.Dangerous(c.ShouldBind(&rev))
	instance, err := model.GetInstanceBy(rev.Module, rev.Identity, rev.RPCPort, rev.HTTPPort)
	errors.Dangerous(err)

	now := time.Now().Unix()
	if instance == nil {
		instance = &model.Instance{
			Identity: rev.Identity,
			Module:   rev.Module,
			RPCPort:  rev.RPCPort,
			HTTPPort: rev.HTTPPort,
			TS:       now,
		}
		errors.Dangerous(instance.Add())
	} else {
		instance.TS = now
		instance.HTTPPort = rev.HTTPPort
		errors.Dangerous(instance.Update())
	}

	renderData(c, "ok", nil)
}

func instanceGets(c *gin.Context) {
	mod := mustQueryStr(c, "mod")
	alive := queryInt(c, "alive", 0)

	instances, err := model.GetAllInstances(mod, alive)

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
		errors.Dangerous(model.DelById(id))
		logger.Infof("[index] %s delete %+v", username, id)
	}
	renderData(c, "ok", nil)
}
