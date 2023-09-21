package router

import (
	"encoding/base64"
	"strings"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) userVariableConfigGets(context *gin.Context) {
	userVariables, err := models.ConfigsGetUserVariable(rt.Ctx)
	ginx.NewRender(context).Data(userVariables, err)
}
func (rt *Router) userVariableConfigAdd(context *gin.Context) {
	var f models.Configs
	ginx.BindJSON(context, &f)
	f.Ckey = strings.TrimSpace(f.Ckey)
	//insert external config. needs to make sure not plaintext for an encrypted type config
	ginx.NewRender(context).Message(models.ConfigsUserVariableInsert(rt.Ctx, f))

}

func (rt *Router) userVariableConfigPut(context *gin.Context) {
	var f models.Configs
	ginx.BindJSON(context, &f)
	f.Id = ginx.UrlParamInt64(context, "id")
	f.Ckey = strings.TrimSpace(f.Ckey)
	//update external config. needs to make sure not plaintext for an encrypted type config
	//updating with struct it will update all fields ("ckey", "cval", "note", "encrypted"), not non-zero fields.
	ginx.NewRender(context).Message(models.ConfigsUserVariableUpdate(rt.Ctx, f))
}

func (rt *Router) userVariableConfigDel(context *gin.Context) {
	id := ginx.UrlParamInt64(context, "id")
	configs, err := models.ConfigGet(rt.Ctx, id)
	ginx.Dangerous(err)

	if configs != nil && configs.External == models.ConfigExternal {
		ginx.NewRender(context).Message(models.ConfigsDel(rt.Ctx, []int64{id}))
	} else {
		ginx.NewRender(context).Message(nil)
	}
}

func (rt *Router) userVariablePublicKey(context *gin.Context) {
	m := map[string]string{"public_key": base64.StdEncoding.EncodeToString(rt.HTTP.RSA.RSAPublicKey)}
	ginx.NewRender(context).Data(m, nil)
}
