package http

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
)

func selfProfileGet(c *gin.Context) {
	renderData(c, loginUser(c), nil)
}

type selfProfileForm struct {
	Nickname string          `json:"nickname"`
	Phone    string          `json:"phone"`
	Email    string          `json:"email"`
	Portrait string          `json:"portrait"`
	Contacts json.RawMessage `json:"contacts"`
}

func selfProfilePut(c *gin.Context) {
	var f selfProfileForm
	bind(c, &f)

	user := loginUser(c)
	user.Nickname = f.Nickname
	user.Phone = f.Phone
	user.Email = f.Email
	user.Portrait = f.Portrait
	user.Contacts = f.Contacts
	user.UpdateAt = time.Now().Unix()
	user.UpdateBy = user.Username

	renderMessage(
		c,
		user.Update(
			"nickname",
			"phone",
			"email",
			"portrait",
			"contacts",
			"update_at",
			"update_by",
		),
	)
}

type selfPasswordForm struct {
	OldPass string `json:"oldpass" binding:"required"`
	NewPass string `json:"newpass" binding:"required"`
}

func selfPasswordPut(c *gin.Context) {
	var f selfPasswordForm
	bind(c, &f)
	renderMessage(c, loginUser(c).ChangePassword(f.OldPass, f.NewPass))
}
