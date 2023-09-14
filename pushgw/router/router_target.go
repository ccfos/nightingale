package router

import (
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) targetUpdate(c *gin.Context) {
	var f idents.TargetUpdate
	ginx.BindJSON(c, &f)

	ginx.NewRender(c).Message(rt.IdentSet.UpdateTargets(f.Lst, f.Now))
}
