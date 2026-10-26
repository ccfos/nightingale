package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/logx"

	"github.com/gin-gonic/gin"
)

// 前端把 token 存在 localStorage（key 为 access_token / refresh_token，见 n9e/fe 的
// src/utils/constant.ts 和登录页），不是 cookie，后端没法靠 302 完成登录，
// 所以返回一段脚本由浏览器自己写入后再跳转。前端改了存储 key 这里要跟着改
const demoLoginPage = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Nightingale</title></head>
<body>
<script>
localStorage.setItem('access_token', %s);
localStorage.setItem('refresh_token', %s);
location.replace(%s);
</script>
</body>
</html>
`

// demoLoginRedirectURL 只允许站内路径，避免配置成外站地址或 javascript: 之类的 scheme
func demoLoginRedirectURL(u string) string {
	if !strings.HasPrefix(u, "/") || strings.HasPrefix(u, "//") || strings.HasPrefix(u, "/\\") {
		return "/"
	}
	return u
}

// demoLogin 演示站免登录入口，不校验任何凭证，直接给配置里的演示账号签发会话
func (rt *Router) demoLogin(c *gin.Context) {
	conf := rt.Center.DemoLogin
	if !conf.Enable {
		c.String(http.StatusNotFound, "demo login is not enabled")
		return
	}

	rctx := c.Request.Context()

	user, err := models.UserGetByUsername(rt.Ctx.WithContext(rctx), conf.Username)
	if err != nil {
		logx.Errorf(rctx, "demo login: get user %q failed: %v", conf.Username, err)
		c.String(http.StatusInternalServerError, "demo login failed")
		return
	}

	// 这个入口谁都能访问，绝不能签出管理员会话
	if user == nil || user.IsDisabled() || user.IsAdmin() {
		logx.Errorf(rctx, "demo login: user %q does not exist, is disabled or is an admin", conf.Username)
		c.String(http.StatusForbidden, "demo login is not available")
		return
	}

	logx.Infof(rctx, "username:%s demo login from:%s", user.Username, c.ClientIP())

	userIdentity := fmt.Sprintf("%d-%s", user.Id, user.Username)

	ts, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, userIdentity)
	if err == nil {
		err = rt.createAuth(rctx, userIdentity, ts)
	}
	if err != nil {
		logx.Errorf(rctx, "demo login: create session for %q failed: %v", conf.Username, err)
		c.String(http.StatusInternalServerError, "demo login failed")
		return
	}

	// json.Marshal 会转义 < > &，结果可以安全地嵌进 <script>
	accessToken, _ := json.Marshal(ts.AccessToken)
	refreshToken, _ := json.Marshal(ts.RefreshToken)
	redirectURL, _ := json.Marshal(demoLoginRedirectURL(conf.RedirectURL))

	// 页面里带着 token，不能被浏览器或 CDN 缓存
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fmt.Sprintf(demoLoginPage, accessToken, refreshToken, redirectURL)))
}
