package router

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

func chartGets(c *gin.Context) {
	lst, err := models.ChartsOf(ginx.QueryInt64(c, "cgid"))
	ginx.NewRender(c).Data(lst, err)
}

func chartAdd(c *gin.Context) {
	var chart models.Chart
	ginx.BindJSON(c, &chart)

	// group_id / configs / weight
	chart.Id = 0
	err := chart.Add()
	ginx.NewRender(c).Data(chart, err)
}

func chartPut(c *gin.Context) {
	var arr []models.Chart
	ginx.BindJSON(c, &arr)

	for i := 0; i < len(arr); i++ {
		ginx.Dangerous(arr[i].Update("configs", "weight", "group_id"))
	}

	ginx.NewRender(c).Message(nil)
}

func chartDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)

	for i := 0; i < len(f.Ids); i++ {
		cg := models.Chart{Id: f.Ids[i]}
		ginx.Dangerous(cg.Del())
	}

	ginx.NewRender(c).Message(nil)
}
