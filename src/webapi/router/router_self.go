package router

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/ormx"
)

func selfProfileGet(c *gin.Context) {
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

func selfProfilePut(c *gin.Context) {
	var f selfProfileForm
	ginx.BindJSON(c, &f)

	user := c.MustGet("user").(*models.User)
	user.Nickname = f.Nickname
	user.Phone = f.Phone
	user.Email = f.Email
	user.Portrait = f.Portrait
	user.Contacts = f.Contacts
	user.UpdateBy = user.Username

	ginx.NewRender(c).Message(user.UpdateAllFields())
}

type selfPasswordForm struct {
	OldPass string `json:"oldpass" binding:"required"`
	NewPass string `json:"newpass" binding:"required"`
}

func selfPasswordPut(c *gin.Context) {
	var f selfPasswordForm
	ginx.BindJSON(c, &f)
	user := c.MustGet("user").(*models.User)
	ginx.NewRender(c).Message(user.ChangePassword(f.OldPass, f.NewPass))
}
