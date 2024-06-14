package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
)

func (rt *Router) operationOfRole(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	role, err := models.RoleGet(rt.Ctx, "id=?", id)
	ginx.Dangerous(err)
	if role == nil {
		ginx.Bomb(http.StatusOK, "role not found")
	}

	if role.Name == "Admin" {
		var lst []string
		for _, ops := range cconf.Operations.Ops {
			lst = append(lst, ops.Ops...)
		}
		ginx.NewRender(c).Data(lst, nil)
		return
	}

	ops, err := models.OperationsOfRole(rt.Ctx, []string{role.Name})
	ginx.NewRender(c).Data(ops, err)
}

func (rt *Router) roleBindOperation(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	role, err := models.RoleGet(rt.Ctx, "id=?", id)
	ginx.Dangerous(err)
	if role == nil {
		ginx.Bomb(http.StatusOK, "role not found")
	}

	if role.Name == "Admin" {
		ginx.Bomb(http.StatusOK, "admin role can not be modified")
	}

	var ops []string
	ginx.BindJSON(c, &ops)

	ginx.NewRender(c).Message(models.RoleOperationBind(rt.Ctx, role.Name, ops))
}

func (rt *Router) operations(c *gin.Context) {
	var ops []cconf.Ops
	for _, v := range rt.Operations.Ops {
		v.Cname = i18n.Sprintf(c.GetHeader("X-Language"), v.Cname)
		ops = append(ops, v)
	}

	ginx.NewRender(c).Data(ops, nil)
}
