package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) operationOfRole(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	role, err := models.RoleGet(rt.Ctx, "id=?", id)
	ginx.Dangerous(err)
	if role == nil {
		ginx.Bomb(http.StatusOK, "role not found")
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
	ginx.NewRender(c).Data(rt.Operations.Ops, nil)
}
