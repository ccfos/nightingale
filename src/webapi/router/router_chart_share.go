package router

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/src/models"
)

func chartShareGets(c *gin.Context) {
	ids := ginx.QueryStr(c, "ids", "")
	lst, err := models.ChartShareGetsByIds(str.IdsInt64(ids, ","))
	ginx.NewRender(c).Data(lst, err)
}

type chartShareForm struct {
	Configs string `json:"configs"`
}

func chartShareAdd(c *gin.Context) {
	username := c.MustGet("username").(string)
	cluster := MustGetCluster(c)

	var forms []chartShareForm
	ginx.BindJSON(c, &forms)

	ids := []int64{}
	now := time.Now().Unix()

	for _, f := range forms {
		chart := models.ChartShare{
			Cluster:  cluster,
			Configs:  f.Configs,
			CreateBy: username,
			CreateAt: now,
		}
		ginx.Dangerous(chart.Add())
		ids = append(ids, chart.Id)
	}

	ginx.NewRender(c).Data(ids, nil)
}
