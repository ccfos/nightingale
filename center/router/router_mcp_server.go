package router

import (
	"context"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/mcp"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
)

// mcpCanManage reports whether the user may manage (edit/delete/test/oauth) the
// server: admins always can; others need a team in common with UserGroupIds.
func mcpCanManage(me *models.User, myGroupIds []int64, obj *models.MCPServer) bool {
	return obj.CanBeManagedBy(me, myGroupIds)
}

// mcpCanUse reports whether the user may use the server's tools in a
// conversation: public servers (Private==0) are usable by everyone; private
// ones only by those who may manage them (admins + owning-team members).
func mcpCanUse(me *models.User, myGroupIds []int64, obj *models.MCPServer) bool {
	return obj.CanBeUsedBy(me, myGroupIds)
}

// mcpCallerCanManage resolves the request's user (and, for non-admins, their
// team ids) and reports whether they may manage obj. obj may be an incoming
// payload, so the same check gates both loading an existing server and
// (re)assigning its ownership on add/update.
func (rt *Router) mcpCallerCanManage(c *gin.Context, obj *models.MCPServer) bool {
	me := c.MustGet("user").(*models.User)
	var gids []int64
	if !me.IsAdmin() {
		var err error
		gids, err = models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)
	}
	return mcpCanManage(me, gids, obj)
}

// mcpServerLoadForManage loads a server by id and 403s unless the caller may
// manage it. Returns the loaded server for the handler to act on.
func (rt *Router) mcpServerLoadForManage(c *gin.Context, id int64) *models.MCPServer {
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}
	if !rt.mcpCallerCanManage(c, obj) {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}
	return obj
}

func (rt *Router) mcpServerGets(c *gin.Context) {
	lst, err := models.MCPServerGets(rt.Ctx)
	ginx.Dangerous(err)

	me := c.MustGet("user").(*models.User)
	var gids []int64
	if !me.IsAdmin() {
		gids, err = models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)
	}

	// Servers with a stored OAuth token: lets the UI flag saved-but-not-yet
	// -authorized oauth servers.
	connectedIds, err := models.MCPServerOAuthConnectedServerIds(rt.Ctx)
	ginx.Dangerous(err)
	connected := make(map[int64]bool, len(connectedIds))
	for _, id := range connectedIds {
		connected[id] = true
	}

	// Non-admins see public servers plus those owned by a team they belong to.
	res := make([]*models.MCPServer, 0, len(lst))
	for _, obj := range lst {
		obj.CanManage = mcpCanManage(me, gids, obj)
		obj.OAuthConnected = connected[obj.Id]
		if me.IsAdmin() || obj.Private == 0 || obj.CanManage {
			if !obj.CanManage {
				// Visible-but-not-manageable (a public server owned by another
				// team): hide its Headers so the auth token isn't leaked to a
				// user who can't edit it.
				obj.MaskSecrets()
			}
			res = append(res, obj)
		}
	}
	ginx.NewRender(c).Data(res, nil)
}

func (rt *Router) mcpServerGet(c *gin.Context) {
	obj := rt.mcpServerLoadForManage(c, ginx.UrlParamInt64(c, "id"))
	obj.CanManage = true
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) mcpServerAdd(c *gin.Context) {
	var obj models.MCPServer
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)
	// Non-admins may only create servers owned by a team they belong to.
	if !rt.mcpCallerCanManage(c, &obj) {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}
	obj.CreatedBy = me.Username
	obj.UpdatedBy = me.Username

	ginx.Dangerous(obj.Create(rt.Ctx))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) mcpServerPut(c *gin.Context) {
	obj := rt.mcpServerLoadForManage(c, ginx.UrlParamInt64(c, "id"))

	var ref models.MCPServer
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)
	// A non-admin must keep the server owned by at least one of their teams, so
	// they can't hand it off to a team they don't belong to and lose control.
	if !rt.mcpCallerCanManage(c, &ref) {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}
	ref.UpdatedBy = me.Username

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, ref))
}

func (rt *Router) mcpServerDel(c *gin.Context) {
	obj := rt.mcpServerLoadForManage(c, ginx.UrlParamInt64(c, "id"))
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

func (rt *Router) mcpServerTest(c *gin.Context) {
	var body struct {
		Id      int64             `json:"id"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	ginx.BindJSON(c, &body)

	var cfg *mcp.ServerConfig
	if body.Id > 0 {
		// Saved server (may be oauth): use its stored config + tokens.
		obj := rt.mcpServerLoadForManage(c, body.Id)
		var err error
		cfg, _, err = rt.mcpServerConfig(obj)
		ginx.Dangerous(err)
	} else {
		if body.URL == "" {
			ginx.Bomb(http.StatusBadRequest, "url is required")
		}
		cfg = &mcp.ServerConfig{
			Name:      "test",
			Transport: mcp.MCPTransportHTTP,
			URL:       body.URL,
			Headers:   body.Headers,
			AuthMode:  mcp.MCPAuthHeader,
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	start := time.Now()
	tools, testErr := mcp.ListToolsForConfig(ctx, cfg)
	durationMs := time.Since(start).Milliseconds()

	result := gin.H{
		"success":     testErr == nil,
		"duration_ms": durationMs,
		"tool_count":  len(tools),
	}
	if testErr != nil {
		result["error"] = testErr.Error()
	}
	ginx.NewRender(c).Data(result, nil)
}

func (rt *Router) mcpServerTools(c *gin.Context) {
	obj := rt.mcpServerLoadForManage(c, ginx.UrlParamInt64(c, "id"))

	cfg, _, err := rt.mcpServerConfig(obj)
	ginx.Dangerous(err)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	ginx.NewRender(c).Data(mcp.ListToolsForConfig(ctx, cfg))
}
