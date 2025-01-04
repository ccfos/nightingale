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
	var (
		role           *models.Role
		err            error
		res            []cconf.SingleOp
		roleOperations []string
	)

	id := ginx.UrlParamInt64(c, "id")
	role, err = models.RoleGet(rt.Ctx, "id=?", id)
	ginx.Dangerous(err)
	if role == nil {
		ginx.Bomb(http.StatusOK, "role not found")
	}

	if role.Name == "Admin" {
		for _, ops := range cconf.Operations.Ops {
			for i := range ops.Ops {
				res = append(res, cconf.SingleOp{
					Cname: i18n.Sprintf(c.GetHeader("X-Language"), ops.Ops[i].Name),
					Name:  ops.Ops[i].Name,
				})
			}
		}
	} else {
		roleOperations, err = models.OperationsOfRole(rt.Ctx, []string{role.Name})
		for i := range roleOperations {
			res = append(res, cconf.SingleOp{
				Cname: i18n.Sprintf(c.GetHeader("X-Language"), roleOperations[i]),
				Name:  roleOperations[i],
			})
		}
	}

	ginx.NewRender(c).Data(res, err)
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
		v.Cname = i18n.Sprintf(c.GetHeader("X-Language"), v.Name)
		for i := range v.Ops {
			v.Ops[i].Cname = i18n.Sprintf(c.GetHeader("X-Language"), v.Ops[i].Name)
		}
		ops = append(ops, v)
	}

	ginx.NewRender(c).Data(ops, nil)
}
