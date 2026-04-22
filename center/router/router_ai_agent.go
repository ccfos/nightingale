package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
)

func (rt *Router) aiAgentGets(c *gin.Context) {
	lst, err := models.AIAgentGets(rt.Ctx)
	ginx.Dangerous(err)

	ids := make([]int64, 0, len(lst))
	for _, obj := range lst {
		ids = append(ids, obj.LLMConfigId)
	}

	configs, err := models.AILLMConfigGetByIds(rt.Ctx, ids)
	ginx.Dangerous(err)

	configMap := make(map[int64]string, len(configs))
	for _, cfg := range configs {
		configMap[cfg.Id] = cfg.Name
	}

	for _, obj := range lst {
		obj.LLMConfigName = configMap[obj.LLMConfigId]
	}

	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) aiAgentGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AIAgentGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai agent not found")
	}

	llmConfig, err := models.AILLMConfigGetById(rt.Ctx, obj.LLMConfigId)
	ginx.Dangerous(err)
	if llmConfig != nil {
		obj.LLMConfigName = llmConfig.Name
	}

	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiAgentAdd(c *gin.Context) {
	var obj models.AIAgent
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)

	ginx.Dangerous(obj.Create(rt.Ctx, me.Username))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) aiAgentPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AIAgentGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai agent not found")
	}

	var ref models.AIAgent
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, me.Username, ref))
}

func (rt *Router) aiAgentDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AIAgentGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai agent not found")
	}
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}
