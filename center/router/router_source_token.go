package router

import (
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// sourceTokenAdd 生成新的源令牌
func (rt *Router) sourceTokenAdd(c *gin.Context) {
	var f models.SourceToken
	ginx.BindJSON(c, &f)

	if f.ExpireAt > 0 && f.ExpireAt <= time.Now().Unix() {
		ginx.Bomb(http.StatusBadRequest, "expire time must be in the future")
	}

	token := uuid.New().String()

	username := c.MustGet("username").(string)

	f.Token = token
	f.CreateBy = username
	f.CreateAt = time.Now().Unix()

	err := f.Add(rt.Ctx)
	ginx.Dangerous(err)

	go models.CleanupExpiredTokens(rt.Ctx)
	ginx.NewRender(c).Data(token, nil)
}
