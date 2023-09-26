package router

import (
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

func (rt *Router) MacroVariableGetDecryptByService(context *gin.Context) {
	decryptMap, decryptErr := models.MacroVariableGetDecryptMap(rt.Ctx, rt.HTTP.RSA.RSAPrivateKey, rt.HTTP.RSA.RSAPassWord)
	ginx.NewRender(context).Data(decryptMap, decryptErr)
}
