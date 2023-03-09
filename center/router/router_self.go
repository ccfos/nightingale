package router

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ormx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) selfProfileGet(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	if user.IsAdmin() {
		user.Admin = true
	}
	ginx.NewRender(c).Data(user, nil)
}

type selfProfileForm struct {
	Nickname string       `json:"nickname"`
	Phone    string       `json:"phone"`
	Email    string       `json:"email"`
	Portrait string       `json:"portrait"`
	Contacts ormx.JSONObj `json:"contacts"`
}

func (rt *Router) selfProfilePut(c *gin.Context) {
	var f selfProfileForm
	ginx.BindJSON(c, &f)

	user := c.MustGet("user").(*models.User)
	user.Nickname = f.Nickname
	user.Phone = f.Phone
	user.Email = f.Email
	user.Portrait = f.Portrait
	user.Contacts = f.Contacts
	user.UpdateBy = user.Username

	ginx.NewRender(c).Message(user.UpdateAllFields(rt.Ctx))
}

type selfPasswordForm struct {
	OldPass string `json:"oldpass" binding:"required"`
	NewPass string `json:"newpass" binding:"required"`
}

func (rt *Router) selfPasswordPut(c *gin.Context) {
	var f selfPasswordForm
	ginx.BindJSON(c, &f)
	user := c.MustGet("user").(*models.User)
	ginx.NewRender(c).Message(user.ChangePassword(rt.Ctx, f.OldPass, f.NewPass))
}
