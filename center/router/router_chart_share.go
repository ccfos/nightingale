package router

import (
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/str"
)

func (rt *Router) chartShareGets(c *gin.Context) {
	ids := ginx.QueryStr(c, "ids", "")
	lst, err := models.ChartShareGetsByIds(rt.Ctx, str.IdsInt64(ids, ","))
	ginx.NewRender(c).Data(lst, err)
}

type chartShareForm struct {
	DatasourceId int64  `json:"datasource_id"`
	Configs      string `json:"configs"`
}

func (rt *Router) chartShareAdd(c *gin.Context) {
	username := c.MustGet("username").(string)

	var forms []chartShareForm
	ginx.BindJSON(c, &forms)

	ids := []int64{}
	now := time.Now().Unix()

	for _, f := range forms {
		chart := models.ChartShare{
			DatasourceId: f.DatasourceId,
			Configs:      f.Configs,
			CreateBy:     username,
			CreateAt:     now,
		}
		ginx.Dangerous(chart.Add(rt.Ctx))
		ids = append(ids, chart.Id)
	}

	ginx.NewRender(c).Data(ids, nil)
}
