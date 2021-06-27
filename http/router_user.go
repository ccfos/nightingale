package http

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/config"
	"github.com/didi/nightingale/v5/models"
)

func userGets(c *gin.Context) {
	limit := queryInt(c, "limit", defaultLimit)
	query := queryStr(c, "query", "")

	total, err := models.UserTotal(query)
	dangerous(err)

	list, err := models.UserGets(query, limit, offset(c, limit))
	dangerous(err)

	admin := loginUser(c).Role == "Admin"

	renderData(c, gin.H{
		"list":  list,
		"total": total,
		"admin": admin,
	}, nil)
}

type userAddForm struct {
	Username string          `json:"username" binding:"required"`
	Password string          `json:"password" binding:"required"`
	Nickname string          `json:"nickname"`
	Phone    string          `json:"phone"`
	Email    string          `json:"email"`
	Portrait string          `json:"portrait"`
	Role     string          `json:"role"`
	Contacts json.RawMessage `json:"contacts"`
}

func userAddPost(c *gin.Context) {
	var f userAddForm
	bind(c, &f)

	password, err := models.CryptoPass(f.Password)
	dangerous(err)

	now := time.Now().Unix()
	username := loginUsername(c)

	u := models.User{
		Username: f.Username,
		Password: password,
		Nickname: f.Nickname,
		Phone:    f.Phone,
		Email:    f.Email,
		Portrait: f.Portrait,
		Role:     f.Role,
		Contacts: f.Contacts,
		CreateAt: now,
		UpdateAt: now,
		CreateBy: username,
		UpdateBy: username,
	}

	if u.Role == "" {
		u.Role = "Standard"
	}

	renderMessage(c, u.Add())
}

func userProfileGet(c *gin.Context) {
	renderData(c, User(urlParamInt64(c, "id")), nil)
}

type userProfileForm struct {
	Nickname string          `json:"nickname"`
	Phone    string          `json:"phone"`
	Email    string          `json:"email"`
	Portrait string          `json:"portrait"`
	Role     string          `json:"role"`
	Status   int             `json:"status"`
	Contacts json.RawMessage `json:"contacts"`
}

func userProfilePut(c *gin.Context) {
	var f userProfileForm
	bind(c, &f)

	target := User(urlParamInt64(c, "id"))
	target.Nickname = f.Nickname
	target.Phone = f.Phone
	target.Email = f.Email
	target.Portrait = f.Portrait
	target.Role = f.Role
	target.Status = f.Status
	target.Contacts = f.Contacts
	target.UpdateAt = time.Now().Unix()
	target.UpdateBy = loginUsername(c)
	renderMessage(
		c,
		target.Update(
			"nickname",
			"phone",
			"email",
			"portrait",
			"role",
			"status",
			"contacts",
			"update_at",
			"update_by",
		),
	)
}

type userPasswordForm struct {
	Password string `json:"password" binding:"required"`
}

func userPasswordPut(c *gin.Context) {
	var f userPasswordForm
	bind(c, &f)

	target := User(urlParamInt64(c, "id"))

	cryptoPass, err := models.CryptoPass(f.Password)
	dangerous(err)

	target.Password = cryptoPass
	target.UpdateAt = time.Now().Unix()
	target.UpdateBy = loginUsername(c)
	renderMessage(c, target.Update("password", "update_at", "update_by"))
}

type userStatusForm struct {
	Status int `json:"status"`
}

func userStatusPut(c *gin.Context) {
	var f userStatusForm
	bind(c, &f)

	target := User(urlParamInt64(c, "id"))
	target.Status = f.Status
	target.UpdateAt = time.Now().Unix()
	target.UpdateBy = loginUsername(c)
	renderMessage(c, target.Update("status", "update_at", "update_by"))
}

func userDel(c *gin.Context) {
	id := urlParamInt64(c, "id")
	target, err := models.UserGet("id=?", id)
	dangerous(err)

	if target == nil {
		renderMessage(c, nil)
		return
	}

	renderMessage(c, target.Del())
}

func contactChannelsGet(c *gin.Context) {
	renderData(c, config.Config.ContactKeys, nil)
}
