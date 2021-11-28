package router

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/server/engine"
	"github.com/didi/nightingale/v5/src/server/reader"
)

type vectorsForm struct {
	PromQL string `json:"promql"`
}

func vectorsPost(c *gin.Context) {
	var f vectorsForm
	ginx.BindJSON(c, &f)

	value, warnings, err := reader.Reader.Client.Query(c.Request.Context(), f.PromQL, time.Now())
	if err != nil {
		c.String(500, "promql:%s error:%v", f.PromQL, err)
		return
	}

	if len(warnings) > 0 {
		c.String(500, "promql:%s warnings:%v", f.PromQL, warnings)
		return
	}

	c.JSON(200, engine.ConvertVectors(value))
}
