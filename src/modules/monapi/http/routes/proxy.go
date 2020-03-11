package routes

import (
	"fmt"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"

	"github.com/didi/nightingale/src/toolkits/address"
)

func transferReq(c *gin.Context) {
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", address.GetHTTPPort("transfer")))
	errors.Dangerous(err)

	proxy := httputil.NewSingleHostReverseProxy(target)
	c.Request.Header.Set("X-Forwarded-Host", c.Request.Header.Get("Host"))

	proxy.ServeHTTP(c.Writer, c.Request)
}

func indexReq(c *gin.Context) {
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", address.GetHTTPPort("index")))
	errors.Dangerous(err)

	proxy := httputil.NewSingleHostReverseProxy(target)
	c.Request.Header.Set("X-Forwarded-Host", c.Request.Header.Get("Host"))

	proxy.ServeHTTP(c.Writer, c.Request)
}
