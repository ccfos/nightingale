package router

import (
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/mcp"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
)

func (rt *Router) mcpServerGets(c *gin.Context) {
	lst, err := models.MCPServerGets(rt.Ctx)
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) mcpServerGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) mcpServerAdd(c *gin.Context) {
	var obj models.MCPServer
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)
	obj.CreatedBy = me.Username
	obj.UpdatedBy = me.Username

	ginx.Dangerous(obj.Create(rt.Ctx))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) mcpServerPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}

	var ref models.MCPServer
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)
	ref.UpdatedBy = me.Username

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, ref))
}

func (rt *Router) mcpServerDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

func (rt *Router) mcpServerTest(c *gin.Context) {
	var body struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	ginx.BindJSON(c, &body)

	if body.URL == "" {
		ginx.Bomb(http.StatusBadRequest, "url is required")
	}

	start := time.Now()
	tools, testErr := mcp.ListToolsHTTP(body.URL, body.Headers)
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
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}

	ginx.NewRender(c).Data(mcp.ListToolsHTTP(obj.URL, obj.Headers))
}
