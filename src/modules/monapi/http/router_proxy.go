package http

import (
	"net/http/httputil"
	"net/url"

	"github.com/didi/nightingale/src/modules/monapi/config"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

func transferReq(c *gin.Context) {
	target, err := url.Parse(config.Get().Proxy.Transfer)
	errors.Dangerous(err)

	proxy := httputil.NewSingleHostReverseProxy(target)
	c.Request.Header.Set("X-Forwarded-Host", c.Request.Header.Get("Host"))

	proxy.ServeHTTP(c.Writer, c.Request)
}

func indexReq(c *gin.Context) {
	target, err := url.Parse(config.Get().Proxy.Index)
	errors.Dangerous(err)

	proxy := httputil.NewSingleHostReverseProxy(target)
	c.Request.Header.Set("X-Forwarded-Host", c.Request.Header.Get("Host"))

	proxy.ServeHTTP(c.Writer, c.Request)
}
