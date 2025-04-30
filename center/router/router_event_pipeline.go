package router

import (
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// 获取事件Pipeline列表
func (rt *Router) eventPipelinesList(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	pipelines, err := models.ListEventPipelines(rt.Ctx)
	ginx.Dangerous(err)

	allTids := make([]int64, 0)
	for _, pipeline := range pipelines {
		allTids = append(allTids, pipeline.TeamIds...)
	}
	ugMap, err := models.UserGroupIdAndNameMap(rt.Ctx, allTids)
	ginx.Dangerous(err)
	for _, pipeline := range pipelines {
		for _, tid := range pipeline.TeamIds {
			pipeline.TeamNames = append(pipeline.TeamNames, ugMap[tid])
		}
	}

	gids, err := models.MyGroupIdsMap(rt.Ctx, me.Id)
	ginx.Dangerous(err)

	if me.IsAdmin() {
		ginx.NewRender(c).Data(pipelines, nil)
		return
	}

	res := make([]*models.EventPipeline, 0)
	for _, pipeline := range pipelines {
		for _, tid := range pipeline.TeamIds {
			if _, ok := gids[tid]; ok {
				res = append(res, pipeline)
				break
			}
		}
	}

	ginx.NewRender(c).Data(res, nil)
}

// 获取单个事件Pipeline详情
func (rt *Router) getEventPipeline(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	id := ginx.UrlParamInt64(c, "id")
	pipeline, err := models.GetEventPipeline(rt.Ctx, id)
	ginx.Dangerous(err)
	ginx.Dangerous(me.CheckGroupPermission(rt.Ctx, pipeline.TeamIds))

	err = pipeline.FillTeamNames(rt.Ctx)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(pipeline, nil)
}

// 创建事件Pipeline
func (rt *Router) addEventPipeline(c *gin.Context) {
	var pipeline models.EventPipeline
	ginx.BindJSON(c, &pipeline)

	user := c.MustGet("user").(*models.User)
	now := time.Now().Unix()
	pipeline.CreatedBy = user.Username
	pipeline.CreatedAt = now
	pipeline.UpdateAt = now
	pipeline.UpdateBy = user.Username

	err := pipeline.Verify()
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	ginx.Dangerous(user.CheckGroupPermission(rt.Ctx, pipeline.TeamIds))
	err = models.CreateEventPipeline(rt.Ctx, &pipeline)
	ginx.NewRender(c).Message(err)
}

// 更新事件Pipeline
func (rt *Router) updateEventPipeline(c *gin.Context) {
	var f models.EventPipeline
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	f.UpdateBy = me.Username
	f.UpdateAt = time.Now().Unix()

	pipeline, err := models.GetEventPipeline(rt.Ctx, f.ID)
	if err != nil {
		ginx.Bomb(http.StatusNotFound, "No such event pipeline")
	}
	ginx.Dangerous(me.CheckGroupPermission(rt.Ctx, pipeline.TeamIds))

	ginx.NewRender(c).Message(pipeline.Update(rt.Ctx, &f))
}

// 删除事件Pipeline
func (rt *Router) deleteEventPipelines(c *gin.Context) {
	var f struct {
		Ids []int64 `json:"ids"`
	}
	ginx.BindJSON(c, &f)

	if len(f.Ids) == 0 {
		ginx.Bomb(http.StatusBadRequest, "ids required")
	}

	me := c.MustGet("user").(*models.User)
	for _, id := range f.Ids {
		pipeline, err := models.GetEventPipeline(rt.Ctx, id)
		ginx.Dangerous(err)
		ginx.Dangerous(me.CheckGroupPermission(rt.Ctx, pipeline.TeamIds))
	}

	err := models.DeleteEventPipelines(rt.Ctx, f.Ids)
	ginx.NewRender(c).Message(err)
}

// 测试事件Pipeline
func (rt *Router) tryRunEventPipeline(c *gin.Context) {
	var f struct {
		EventID        int64                `json:"event_id"`
		PipelineConfig models.EventPipeline `json:"pipeline_config"`
	}
	ginx.BindJSON(c, &f)

	// 获取告警事件
	event, err := models.AlertCurEventGetById(rt.Ctx, f.EventID)
	if err != nil || event == nil {
		ginx.Bomb(http.StatusBadRequest, "event not found")
	}

	// 在实际应用中，这里需要执行事件处理器并返回处理后的事件
	// 模拟处理结果返回
	ginx.NewRender(c).Data(event, nil)
}

// 测试事件处理器
func (rt *Router) tryRunEventProcessor(c *gin.Context) {
	var f struct {
		EventID         int64 `json:"event_id"`
		ProcessorConfig struct {
			Type   string      `json:"type"`
			Config interface{} `json:"config"`
		} `json:"processor_config"`
	}
	ginx.BindJSON(c, &f)

	// 获取告警事件
	event, err := models.AlertCurEventGetById(rt.Ctx, f.EventID)
	if err != nil || event == nil {
		ginx.Bomb(http.StatusBadRequest, "event not found")
	}

	// 在实际应用中，这里需要执行单个处理器并返回处理后的事件
	// 模拟处理结果返回
	ginx.NewRender(c).Data(event, nil)
}
