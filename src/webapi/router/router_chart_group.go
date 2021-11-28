package router

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

func chartGroupGets(c *gin.Context) {
	objs, err := models.ChartGroupsOf(ginx.QueryInt64(c, "did"))
	ginx.NewRender(c).Data(objs, err)
}

func chartGroupAdd(c *gin.Context) {
	var cg models.ChartGroup
	ginx.BindJSON(c, &cg)

	// dashboard_id / name / weight
	cg.Id = 0
	err := cg.Add()
	ginx.NewRender(c).Data(cg, err)
}

func chartGroupPut(c *gin.Context) {
	var arr []models.ChartGroup
	ginx.BindJSON(c, &arr)

	for i := 0; i < len(arr); i++ {
		ginx.Dangerous(arr[i].Update("name", "weight", "dashboard_id"))
	}

	ginx.NewRender(c).Message(nil)
}

func chartGroupDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)

	for i := 0; i < len(f.Ids); i++ {
		cg := models.ChartGroup{Id: f.Ids[i]}
		ginx.Dangerous(cg.Del())
	}

	ginx.NewRender(c).Message(nil)
}
