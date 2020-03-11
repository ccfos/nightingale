package routes

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/monapi/config"
)

func userListGet(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")

	total, err := model.UserTotal(query)
	errors.Dangerous(err)

	list, err := model.UserGets(query, limit, offset(c, limit, total))
	errors.Dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type userAddForm struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Dispname string `json:"dispname"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Im       string `json:"im"`
	IsRoot   int    `json:"is_root"`
}

func userAddPost(c *gin.Context) {
	loginRoot(c)

	var f userAddForm
	errors.Dangerous(c.ShouldBind(&f))

	u := model.User{
		Username: f.Username,
		Password: config.CryptoPass(f.Password),
		Dispname: f.Dispname,
		Phone:    f.Phone,
		Email:    f.Email,
		Im:       f.Im,
		IsRoot:   f.IsRoot,
	}

	renderMessage(c, u.Save())
}

func userInviteGet(c *gin.Context) {
	loginUser := cookieUsername(c)
	if loginUser == "" {
		errors.Bomb("unauthorized")
	}

	token := config.CryptoPass(fmt.Sprint(time.Now().UnixNano()))
	err := model.InviteAdd(token, loginUser)
	renderData(c, token, err)
}

type userInviteForm struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Dispname string `json:"dispname"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Im       string `json:"im"`
	Token    string `json:"token" binding:"required"`
}

func userInvitePost(c *gin.Context) {
	var f userInviteForm
	errors.Dangerous(c.ShouldBind(&f))

	inv, err := model.InviteGet("token", f.Token)
	errors.Dangerous(err)

	if inv.Expire < time.Now().Unix() {
		errors.Dangerous("invite url already expired")
	}

	u := model.User{
		Username: f.Username,
		Password: config.CryptoPass(f.Password),
		Dispname: f.Dispname,
		Phone:    f.Phone,
		Email:    f.Email,
		Im:       f.Im,
	}

	renderMessage(c, u.Save())
}

func userProfileGet(c *gin.Context) {
	renderData(c, mustUser(urlParamInt64(c, "id")), nil)
}

type userProfileForm struct {
	Dispname string `json:"dispname"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Im       string `json:"im"`
	IsRoot   int    `json:"is_root"`
}

func userProfilePut(c *gin.Context) {
	loginRoot(c)

	var f userProfileForm
	errors.Dangerous(c.ShouldBind(&f))

	target := mustUser(urlParamInt64(c, "id"))
	target.Dispname = f.Dispname
	target.Phone = f.Phone
	target.Email = f.Email
	target.Im = f.Im
	target.IsRoot = f.IsRoot
	renderMessage(c, target.Update("dispname", "phone", "email", "im", "is_root"))
}

type userPasswordForm struct {
	Password string `json:"password" binding:"required"`
}

func userPasswordPut(c *gin.Context) {
	loginRoot(c)

	var f userPasswordForm
	errors.Dangerous(c.ShouldBind(&f))

	target := mustUser(urlParamInt64(c, "id"))
	target.Password = config.CryptoPass(f.Password)
	renderMessage(c, target.Update("password"))
}

func userDel(c *gin.Context) {
	loginRoot(c)

	id := urlParamInt64(c, "id")
	target, err := model.UserGet("id", id)
	errors.Dangerous(err)

	if target == nil {
		renderMessage(c, nil)
		return
	}

	if target.Username == "root" {
		errors.Bomb("cannot delete root user")
	}

	renderMessage(c, target.Del())
}
