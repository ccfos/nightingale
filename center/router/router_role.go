package router

import (
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) rolesGets(c *gin.Context) {
	lst, err := models.RoleGetsAll(rt.Ctx)
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) permsGets(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	lst, err := models.OperationsOfRole(rt.Ctx, strings.Fields(user.Roles))
	ginx.NewRender(c).Data(lst, err)
}

// 创建角色
func (rt *Router) roleAdd(c *gin.Context) {
	var f models.Role
	ginx.BindJSON(c, &f)

	err := f.Add(rt.Ctx)
	ginx.NewRender(c).Message(err)
}

// 更新角色
func (rt *Router) rolePut(c *gin.Context) {
	var f models.Role
	ginx.BindJSON(c, &f)
	oldRule, err := models.RoleGet(rt.Ctx, "id=?", f.Id)
	ginx.Dangerous(err)

	if oldRule == nil {
		ginx.Bomb(http.StatusOK, "role not found")
	}

	if oldRule.Name == "Admin" {
		ginx.Bomb(http.StatusOK, "admin role can not be modified")
	}

	if oldRule.Name != f.Name {
		// name changed, check duplication
		num, err := models.RoleCount(rt.Ctx, "name=? and id<>?", f.Name, oldRule.Id)
		ginx.Dangerous(err)

		if num > 0 {
			ginx.Bomb(http.StatusOK, "role name already exists")
		}
	}

	oldRule.Name = f.Name
	oldRule.Note = f.Note

	ginx.NewRender(c).Message(oldRule.Update(rt.Ctx, "name", "note"))
}

func (rt *Router) roleDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	target, err := models.RoleGet(rt.Ctx, "id=?", id)
	ginx.Dangerous(err)

	if target.Name == "Admin" {
		ginx.Bomb(http.StatusOK, "admin role can not be modified")
	}

	if target == nil {
		ginx.NewRender(c).Message(nil)
		return
	}

	ginx.NewRender(c).Message(target.Del(rt.Ctx))
}

// 角色列表
func (rt *Router) roleGets(c *gin.Context) {
	lst, err := models.RoleGetsAll(rt.Ctx)
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) allPerms(c *gin.Context) {
	roles, err := models.RoleGetsAll(rt.Ctx)
	ginx.Dangerous(err)
	m := make(map[string][]string)
	for _, r := range roles {
		lst, err := models.OperationsOfRole(rt.Ctx, strings.Fields(r.Name))
		if err != nil {
			continue
		}
		m[r.Name] = lst
	}

	ginx.NewRender(c).Data(m, err)
}
