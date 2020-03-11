package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"

	"github.com/didi/nightingale/src/modules/monapi/config"
)

func selfProfileGet(c *gin.Context) {
	renderData(c, loginUser(c), nil)
}

type selfProfileForm struct {
	Dispname string `json:"dispname"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Im       string `json:"im"`
}

func selfProfilePut(c *gin.Context) {
	var f selfProfileForm
	errors.Dangerous(c.ShouldBind(&f))

	user := loginUser(c)
	user.Dispname = f.Dispname
	user.Phone = f.Phone
	user.Email = f.Email
	user.Im = f.Im

	renderMessage(c, user.Update("dispname", "phone", "email", "im"))
}

type selfPasswordForm struct {
	OldPass string `json:"oldpass" binding:"required"`
	NewPass string `json:"newpass" binding:"required"`
}

func selfPasswordPut(c *gin.Context) {
	var f selfPasswordForm
	errors.Dangerous(c.ShouldBind(&f))

	oldpass := config.CryptoPass(f.OldPass)
	newpass := config.CryptoPass(f.NewPass)

	user := loginUser(c)
	if user.Password != oldpass {
		errors.Bomb("old password error")
	}

	user.Password = newpass
	renderMessage(c, user.Update("password"))
}
