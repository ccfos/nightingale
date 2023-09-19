package router

import (
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) userVariableConfigGets(context *gin.Context) {
	query := strings.TrimSpace(ginx.QueryStr(context, "query", ""))
	userVariables, err := models.ConfigsGetUserVariable(rt.Ctx, query)
	ginx.NewRender(context).Data(userVariables, err)
}
func (rt *Router) userVariableConfigAdd(context *gin.Context) {
	var f models.Configs
	ginx.BindJSON(context, &f)
	f.Ckey = strings.TrimSpace(f.Ckey)
	//insert external config. needs to make sure not plaintext for an encrypted type config
	objs, err := models.ConfigsSelectByCkey(rt.Ctx, f.Ckey)
	ginx.Dangerous(err)
	if len(objs) > 0 {
		ginx.NewRender(context).Message(fmt.Errorf("duplicate ckey found: '%s'", f.Ckey))
	} else {
		ginx.NewRender(context).Message(models.ConfigsSetPlus(rt.Ctx, f, &rt.HTTP.RSA))
	}
}

func (rt *Router) userVariableConfigPut(context *gin.Context) {
	var f models.Configs
	ginx.BindJSON(context, &f)
	f.Ckey = strings.TrimSpace(f.Ckey)
	//insert external config. needs to make sure not plaintext for an encrypted type config
	objs, err := models.ConfigsSelectByCkey(rt.Ctx, f.Ckey)
	ginx.Dangerous(err)
	if len(objs) == 0 {
		ginx.NewRender(context).Message(fmt.Errorf("not found ckey: '%s'", f.Ckey))
	} else {
		//update external config. needs to make sure not plaintext for an encrypted type config
		//updating with struct it will update all fields ("cval", "note", "external", "encrypted"),not non-zero fields.
		ginx.NewRender(context).Message(models.ConfigsSetPlus(rt.Ctx, f, &rt.HTTP.RSA))
	}
}

func (rt *Router) userVariableConfigDel(context *gin.Context) {
	id := ginx.UrlParamInt64(context, "id")
	configs, err := models.ConfigGet(rt.Ctx, id)
	ginx.Dangerous(err)

	if configs != nil && configs.External == models.Config_External {
		ginx.NewRender(context).Message(models.ConfigsDel(rt.Ctx, []int64{id}))
	} else {
		ginx.NewRender(context).Message(nil)
	}
}
