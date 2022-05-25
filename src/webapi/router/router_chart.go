package router

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

type ChartFront struct {
	Id 	string `json:"id"`
	GroupId int64  `json:"group_id"`
	Configs string `json:"configs"`
	Weight  int    `json:"weight"`
}

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
	var arr_tmp []ChartFront
	ginx.BindJSON(c, &arr_tmp)

	for i := 0; i < len(arr_tmp); i++ {
		if len(arr_tmp[i].Id) > 0 {
			chartitem := models.Chart{Cid:arr_tmp[i].Id,GroupId:arr_tmp[i].GroupId,Configs:arr_tmp[i].Configs,Weight:arr_tmp[i].Weight}
			cg,err := models.GetChartByCid(chartitem.Cid)
			if err != nil{
				continue;
			}
			if cg.Id > 0 {
				ginx.Dangerous(chartitem.Update("cid","configs", "weight", "group_id"))
			}else {
				chartitem.Id = 0
				chartitem.Add()
			}
		}
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
