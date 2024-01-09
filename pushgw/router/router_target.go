package router

import (
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) targetUpdate(c *gin.Context) {
	var f idents.TargetUpdate
	ginx.BindJSON(c, &f)

	m := make(map[string]struct{})
	for _, ident := range f.Lst {
		m[ident] = struct{}{}
	}

	rt.IdentSet.MSet(m)
	ginx.NewRender(c).Message(nil)
}
