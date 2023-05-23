package router

import (
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ormx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) userFindAll(c *gin.Context) {
	list, err := models.UserGetAll(rt.Ctx)
	ginx.NewRender(c).Data(list, err)
}

func (rt *Router) userGets(c *gin.Context) {
	limit := ginx.QueryInt(c, "limit", 20)
	query := ginx.QueryStr(c, "query", "")

	total, err := models.UserTotal(rt.Ctx, query)
	ginx.Dangerous(err)

	list, err := models.UserGets(rt.Ctx, query, limit, ginx.Offset(c, limit))
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

func (rt *Router) userAddPost(c *gin.Context) {
	var f userAddForm
	ginx.BindJSON(c, &f)

	password, err := models.CryptoPass(rt.Ctx, f.Password)
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

	ginx.NewRender(c).Message(u.Add(rt.Ctx))
}

func (rt *Router) userProfileGet(c *gin.Context) {
	user := User(rt.Ctx, ginx.UrlParamInt64(c, "id"))
	ginx.NewRender(c).Data(user, nil)
}

type userProfileForm struct {
	Nickname string       `json:"nickname"`
	Phone    string       `json:"phone"`
	Email    string       `json:"email"`
	Roles    []string     `json:"roles"`
	Contacts ormx.JSONObj `json:"contacts"`
}

func (rt *Router) userProfilePut(c *gin.Context) {
	var f userProfileForm
	ginx.BindJSON(c, &f)

	if len(f.Roles) == 0 {
		ginx.Bomb(http.StatusBadRequest, "roles empty")
	}

	target := User(rt.Ctx, ginx.UrlParamInt64(c, "id"))
	target.Nickname = f.Nickname
	target.Phone = f.Phone
	target.Email = f.Email
	target.Roles = strings.Join(f.Roles, " ")
	target.Contacts = f.Contacts
	target.UpdateBy = c.MustGet("username").(string)

	ginx.NewRender(c).Message(target.UpdateAllFields(rt.Ctx))
}

type userPasswordForm struct {
	Password string `json:"password" binding:"required"`
}

func (rt *Router) userPasswordPut(c *gin.Context) {
	var f userPasswordForm
	ginx.BindJSON(c, &f)

	target := User(rt.Ctx, ginx.UrlParamInt64(c, "id"))

	cryptoPass, err := models.CryptoPass(rt.Ctx, f.Password)
	ginx.Dangerous(err)

	ginx.NewRender(c).Message(target.UpdatePassword(rt.Ctx, cryptoPass, c.MustGet("username").(string)))
}

func (rt *Router) userDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	target, err := models.UserGetById(rt.Ctx, id)
	ginx.Dangerous(err)

	if target == nil {
		ginx.NewRender(c).Message(nil)
		return
	}

	ginx.NewRender(c).Message(target.Del(rt.Ctx))
}
