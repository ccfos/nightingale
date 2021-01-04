package http

import (
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/auth"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/gin-gonic/gin"
)

func selfProfileGet(c *gin.Context) {
	renderData(c, loginUser(c), nil)
}

type selfProfileForm struct {
	Dispname string `json:"dispname"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Im       string `json:"im"`
	Portrait string `json:"portrait"`
	Intro    string `json:"intro"`
}

func selfProfilePut(c *gin.Context) {
	var f selfProfileForm
	bind(c, &f)

	user := loginUser(c)
	user.Dispname = f.Dispname
	user.Phone = f.Phone
	user.Email = f.Email
	user.Im = f.Im
	user.Portrait = f.Portrait
	user.Intro = f.Intro

	renderMessage(c, user.Update("dispname", "phone", "email", "im", "portrait", "intro"))
}

type selfPasswordForm struct {
	Username string `json:"username" binding:"required"`
	OldPass  string `json:"oldpass" binding:"required"`
	NewPass  string `json:"newpass" binding:"required"`
}

func selfPasswordPut(c *gin.Context) {
	var f selfPasswordForm
	bind(c, &f)

	err := func() error {
		user, err := models.UserMustGet("username=?", f.Username)
		if err != nil {
			return err
		}
		oldpass, err := models.CryptoPass(f.OldPass)
		if err != nil {
			return err
		}
		if user.Password != oldpass {
			return _e("Incorrect old password")
		}

		return auth.ChangePassword(user, f.NewPass)
	}()

	renderMessage(c, err)
}

func selfTokenGets(c *gin.Context) {
	objs, err := models.UserTokenGets("user_id=?", loginUser(c).Id)
	renderData(c, objs, err)
}

func selfTokenPost(c *gin.Context) {
	user := loginUser(c)
	obj, err := models.UserTokenNew(user.Id, user.Username)
	renderData(c, obj, err)
}

type selfTokenForm struct {
	Token string `json:"token"`
}

func selfTokenPut(c *gin.Context) {
	user := loginUser(c)

	var f selfTokenForm
	bind(c, &f)

	obj, err := models.UserTokenReset(user.Id, f.Token)
	renderData(c, obj, err)
}

func permGlobalOps(c *gin.Context) {
	user := loginUser(c)
	operations := make(map[string]struct{})

	if user.IsRoot == 1 {
		for _, system := range config.GlobalOps {
			for _, group := range system.Groups {
				for _, op := range group.Ops {
					operations[op.En] = struct{}{}
				}
			}
		}

		renderData(c, operations, nil)
		return
	}

	roleIds, err := models.RoleIdsGetByUserId(user.Id)
	dangerous(err)

	ops, err := models.OperationsOfRoles(roleIds)
	dangerous(err)

	for _, op := range ops {
		operations[op] = struct{}{}
	}

	renderData(c, operations, err)
}

func v1PermGlobalOps(c *gin.Context) {
	user, err := models.UserGet("username=?", queryStr(c, "username"))
	dangerous(err)

	operations := make(map[string]struct{})

	if user.IsRoot == 1 {
		for _, system := range config.GlobalOps {
			for _, group := range system.Groups {
				for _, op := range group.Ops {
					operations[op.En] = struct{}{}
				}
			}
		}

		renderData(c, operations, nil)
		return
	}

	roleIds, err := models.RoleIdsGetByUserId(user.Id)
	dangerous(err)

	ops, err := models.OperationsOfRoles(roleIds)
	dangerous(err)

	for _, op := range ops {
		operations[op] = struct{}{}
	}

	renderData(c, operations, err)
}
