package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) builtinComponentsAdd(c *gin.Context) {
	var lst []models.BuiltinComponent
	ginx.BindJSON(c, &lst)

	username := Username(c)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		if err := lst[i].Add(rt.Ctx, username); err != nil {
			reterr[lst[i].Ident] = err.Error()
		}
	}

	ginx.NewRender(c).Data(reterr, nil)
}

func (rt *Router) builtinComponentsGets(c *gin.Context) {
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)

	bc, err := models.BuiltinComponentGets(rt.Ctx, query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	total, err := models.BuiltinComponentCount(rt.Ctx, query)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"list":  bc,
		"total": total,
	}, nil)
}

func (rt *Router) builtinComponentsPut(c *gin.Context) {
	var req models.BuiltinComponent
	ginx.BindJSON(c, &req)

	bc, err := models.BuiltinComponentGet(rt.Ctx, "id = ?", req.ID)
	ginx.Dangerous(err)

	if bc == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such builtin component")
		return
	}

	username := Username(c)
	req.UpdatedBy = username

	ginx.NewRender(c).Message(bc.Update(rt.Ctx, req))
}

func (rt *Router) builtinComponentsDel(c *gin.Context) {
	var req idsForm
	ginx.BindJSON(c, &req)

	req.Verify()

	ginx.NewRender(c).Message(models.BuiltinComponentDels(rt.Ctx, req.Ids))
}
