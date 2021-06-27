package http

import (
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

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
