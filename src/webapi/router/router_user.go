package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/ormx"
)

func userFindAll(c *gin.Context) {
	limit := ginx.QueryInt(c, "limit", 20)
	query := ginx.QueryStr(c, "query", "")

	total, err := models.UserTotal(query)
	ginx.Dangerous(err)

	list, err := models.UserGets(query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func userGets(c *gin.Context) {
	limit := ginx.QueryInt(c, "limit", 20)
	query := ginx.QueryStr(c, "query", "")

	total, err := models.UserTotal(query)
	ginx.Dangerous(err)

	list, err := models.UserGets(query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	user := c.MustGet("user").(*models.User)

	ginx.NewRender(c).Data(gin.H{
		"list":  list,
		"total": total,
		"admin": user.IsAdmin(),
	}, nil)
}

type userAddForm struct {
	Username string       `json:"username" binding:"required"`
	Password string       `json:"password" binding:"required"`
	Nickname string       `json:"nickname"`
	Phone    string       `json:"phone"`
	Email    string       `json:"email"`
	Portrait string       `json:"portrait"`
	Roles    []string     `json:"roles" binding:"required"`
	Contacts ormx.JSONObj `json:"contacts"`
}

func userAddPost(c *gin.Context) {
	var f userAddForm
	ginx.BindJSON(c, &f)

	password, err := models.CryptoPass(f.Password)
	ginx.Dangerous(err)

	if len(f.Roles) == 0 {
		ginx.Bomb(http.StatusBadRequest, "roles empty")
	}

	user := c.MustGet("user").(*models.User)

	u := models.User{
		Username: f.Username,
		Password: password,
		Nickname: f.Nickname,
		Phone:    f.Phone,
		Email:    f.Email,
		Portrait: f.Portrait,
		Roles:    strings.Join(f.Roles, " "),
		Contacts: f.Contacts,
		CreateBy: user.Username,
		UpdateBy: user.Username,
	}

	ginx.NewRender(c).Message(u.Add())
}

func userProfileGet(c *gin.Context) {
	user := User(ginx.UrlParamInt64(c, "id"))
	ginx.NewRender(c).Data(user, nil)
}

type userProfileForm struct {
	Nickname string       `json:"nickname"`
	Phone    string       `json:"phone"`
	Email    string       `json:"email"`
	Roles    []string     `json:"roles"`
	Contacts ormx.JSONObj `json:"contacts"`
}

func userProfilePut(c *gin.Context) {
	var f userProfileForm
	ginx.BindJSON(c, &f)

	if len(f.Roles) == 0 {
		ginx.Bomb(http.StatusBadRequest, "roles empty")
	}

	target := User(ginx.UrlParamInt64(c, "id"))
	target.Nickname = f.Nickname
	target.Phone = f.Phone
	target.Email = f.Email
	target.Roles = strings.Join(f.Roles, " ")
	target.Contacts = f.Contacts
	target.UpdateBy = c.MustGet("username").(string)

	ginx.NewRender(c).Message(target.UpdateAllFields())
}

type userPasswordForm struct {
	Password string `json:"password" binding:"required"`
}

func userPasswordPut(c *gin.Context) {
	var f userPasswordForm
	ginx.BindJSON(c, &f)

	target := User(ginx.UrlParamInt64(c, "id"))

	cryptoPass, err := models.CryptoPass(f.Password)
	ginx.Dangerous(err)

	ginx.NewRender(c).Message(target.UpdatePassword(cryptoPass, c.MustGet("username").(string)))
}

func userDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	target, err := models.UserGetById(id)
	ginx.Dangerous(err)

	if target == nil {
		ginx.NewRender(c).Message(nil)
		return
	}

	ginx.NewRender(c).Message(target.Del())
}
