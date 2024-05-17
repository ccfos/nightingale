package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) builtinPayloadsAdd(c *gin.Context) {
	var lst []models.BuiltinPayload
	ginx.BindJSON(c, &lst)

	username := Username(c)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		if err := lst[i].Add(rt.Ctx, username); err != nil {
			reterr[lst[i].Name] = err.Error()
		}
	}

	ginx.NewRender(c).Data(reterr, nil)
}

func (rt *Router) builtinPayloadsGets(c *gin.Context) {
	typ := ginx.QueryStr(c, "type", "")
	component := ginx.QueryStr(c, "component", "")
	cate := ginx.QueryStr(c, "cate", "")
	name := ginx.QueryStr(c, "name", "")
	limit := ginx.QueryInt(c, "limit", 20)

	lst, err := models.BuiltinPayloadGets(rt.Ctx, typ, component, cate, name, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)
	m := make(map[string][]*models.BuiltinPayload)
	for _, v := range lst {
		if _, ok := m[v.Cate]; !ok {
			m[v.Cate] = make([]*models.BuiltinPayload, 0)
		}
		m[v.Cate] = append(m[v.Cate], v)
	}

	ginx.NewRender(c).Data(m, nil)
}

func (rt *Router) builtinPayloadGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")

	bp, err := models.BuiltinPayloadGet(rt.Ctx, "id = ?", id)
	if err != nil {
		ginx.Bomb(http.StatusInternalServerError, err.Error())
	}
	if bp == nil {
		ginx.Bomb(http.StatusNotFound, "builtin payload not found")
	}

	ginx.NewRender(c).Data(bp, nil)
}

func (rt *Router) builtinPayloadsPut(c *gin.Context) {
	var req models.BuiltinPayload
	ginx.BindJSON(c, &req)

	bp, err := models.BuiltinPayloadGet(rt.Ctx, "id = ?", req.ID)
	ginx.Dangerous(err)

	if bp == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such builtin payload")
		return
	}

	username := Username(c)
	req.UpdatedBy = username

	ginx.NewRender(c).Message(bp.Update(rt.Ctx, req))
}

func (rt *Router) builtinPayloadsDel(c *gin.Context) {
	var req idsForm
	ginx.BindJSON(c, &req)

	req.Verify()

	ginx.NewRender(c).Message(models.BuiltinPayloadDels(rt.Ctx, req.Ids))
}
