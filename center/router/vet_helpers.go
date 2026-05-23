package router

import (
	"errors"

	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/i18nx"
	"github.com/gin-gonic/gin"
)

func bombMsg(code int, msg string) {
	ginx.Bomb(code, "%s", msg)
}

func bombErr(code int, err error) {
	if err != nil {
		ginx.Bomb(code, "%s", err.Error())
	}
}

func translateText(lang, key string) string {
	return i18nx.Translate(lang, key)
}

func requestLang(c *gin.Context) string {
	return c.GetHeader("X-Language")
}

func translate(c *gin.Context, key string) string {
	return i18nx.Translate(requestLang(c), key)
}

func translatef(c *gin.Context, format string, args ...interface{}) string {
	return i18nx.Translatef(requestLang(c), format, args...)
}

func bombI18n(c *gin.Context, code int, format string, args ...interface{}) {
	bombMsg(code, translatef(c, format, args...))
}

func newMessageError(msg string) error {
	return errors.New(msg)
}
