package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/alert/pipeline/engine"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
	"github.com/toolkits/pkg/logger"
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
		// 兼容处理：自动填充工作流字段
		pipeline.FillWorkflowFields()
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

	// 兼容处理：自动填充工作流字段
	pipeline.FillWorkflowFields()

	ginx.NewRender(c).Data(pipeline, nil)
}

// 创建事件Pipeline
func (rt *Router) addEventPipeline(c *gin.Context) {
	var pipeline models.EventPipeline
	ginx.BindJSON(c, &pipeline)

	user := c.MustGet("user").(*models.User)
	now := time.Now().Unix()
	pipeline.CreateBy = user.Username
	pipeline.CreateAt = now
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
		EventId        int64                `json:"event_id"`
		PipelineConfig models.EventPipeline `json:"pipeline_config"`
		EnvVariables   map[string]string    `json:"env_variables,omitempty"`
	}

	ginx.BindJSON(c, &f)

	hisEvent, err := models.AlertHisEventGetById(rt.Ctx, f.EventId)
	if err != nil || hisEvent == nil {
		ginx.Bomb(http.StatusBadRequest, "event not found")
	}
	event := hisEvent.ToCur()

	lang := c.GetHeader("X-Language")
	me := c.MustGet("user").(*models.User)

	// 统一使用工作流引擎执行（兼容线性模式和工作流模式）
	workflowEngine := engine.NewWorkflowEngine(rt.Ctx)

	triggerCtx := &models.WorkflowTriggerContext{
		Mode:         models.TriggerModeAPI,
		TriggerBy:    me.Username,
		EnvOverrides: f.EnvVariables,
	}

	resultEvent, result, err := workflowEngine.Execute(&f.PipelineConfig, event, triggerCtx)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, "pipeline execute error: %v", err)
	}

	m := map[string]interface{}{
		"event":        resultEvent,
		"result":       i18n.Sprintf(lang, result.Message),
		"status":       result.Status,
		"node_results": result.NodeResults,
	}

	if resultEvent == nil {
		m["result"] = i18n.Sprintf(lang, "event is dropped")
	}

	ginx.NewRender(c).Data(m, nil)
}

// 测试事件处理器
func (rt *Router) tryRunEventProcessor(c *gin.Context) {
	var f struct {
		EventId         int64                  `json:"event_id"`
		ProcessorConfig models.ProcessorConfig `json:"processor_config"`
	}
	ginx.BindJSON(c, &f)

	hisEvent, err := models.AlertHisEventGetById(rt.Ctx, f.EventId)
	if err != nil || hisEvent == nil {
		ginx.Bomb(http.StatusBadRequest, "event not found")
	}
	event := hisEvent.ToCur()

	processor, err := models.GetProcessorByType(f.ProcessorConfig.Typ, f.ProcessorConfig.Config)
	if err != nil {
		ginx.Bomb(200, "get processor err: %+v", err)
	}
	wfCtx := &models.WorkflowContext{
		Event: event,
		Vars:  make(map[string]interface{}),
	}
	wfCtx, res, err := processor.Process(rt.Ctx, wfCtx)
	if err != nil {
		ginx.Bomb(200, "processor err: %+v", err)
	}

	lang := c.GetHeader("X-Language")
	ginx.NewRender(c).Data(map[string]interface{}{
		"event":  wfCtx.Event,
		"result": i18n.Sprintf(lang, res),
	}, nil)
}

func (rt *Router) tryRunEventProcessorByNotifyRule(c *gin.Context) {
	var f struct {
		EventId         int64                   `json:"event_id"`
		PipelineConfigs []models.PipelineConfig `json:"pipeline_configs"`
	}
	ginx.BindJSON(c, &f)

	hisEvent, err := models.AlertHisEventGetById(rt.Ctx, f.EventId)
	if err != nil || hisEvent == nil {
		ginx.Bomb(http.StatusBadRequest, "event not found")
	}
	event := hisEvent.ToCur()

	pids := make([]int64, 0)
	for _, pc := range f.PipelineConfigs {
		if pc.Enable {
			pids = append(pids, pc.PipelineId)
		}
	}

	pipelines, err := models.GetEventPipelinesByIds(rt.Ctx, pids)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, "processors not found")
	}

	wfCtx := &models.WorkflowContext{
		Event: event,
		Vars:  make(map[string]interface{}),
	}
	for _, pl := range pipelines {
		for _, p := range pl.ProcessorConfigs {
			processor, err := models.GetProcessorByType(p.Typ, p.Config)
			if err != nil {
				ginx.Bomb(http.StatusBadRequest, "get processor: %+v err: %+v", p, err)
			}

			wfCtx, _, err = processor.Process(rt.Ctx, wfCtx)
			if err != nil {
				ginx.Bomb(http.StatusBadRequest, "processor: %+v err: %+v", p, err)
			}
			if wfCtx == nil || wfCtx.Event == nil {
				lang := c.GetHeader("X-Language")
				ginx.NewRender(c).Data(map[string]interface{}{
					"event":  nil,
					"result": i18n.Sprintf(lang, "event is dropped"),
				}, nil)
				return
			}
		}
	}

	ginx.NewRender(c).Data(wfCtx.Event, nil)
}

func (rt *Router) eventPipelinesListByService(c *gin.Context) {
	pipelines, err := models.ListEventPipelines(rt.Ctx)
	ginx.NewRender(c).Data(pipelines, err)
}

// ========== API 触发工作流 ==========
type EventPipelineRequest struct {
	// 事件数据（可选，如果不传则使用空事件）
	Event *models.AlertCurEvent `json:"event,omitempty"`
	// 环境变量覆盖
	EnvOverrides map[string]string `json:"env_overrides,omitempty"`

	Username string `json:"username,omitempty"`
}

// executePipelineTrigger 执行 Pipeline 触发的公共逻辑
func (rt *Router) executePipelineTrigger(pipeline *models.EventPipeline, req *EventPipelineRequest, triggerBy string) (string, error) {
	// 准备事件数据
	var event *models.AlertCurEvent
	if req.Event != nil {
		event = req.Event
	} else {
		// 创建空事件
		event = &models.AlertCurEvent{
			TriggerTime: time.Now().Unix(),
		}
	}

	// 校验必填环境变量
	if err := pipeline.ValidateEnvVariables(req.EnvOverrides); err != nil {
		return "", fmt.Errorf("env validation failed: %v", err)
	}

	// 生成执行ID
	executionID := uuid.New().String()

	// 创建触发上下文
	triggerCtx := &models.WorkflowTriggerContext{
		Mode:         models.TriggerModeAPI,
		TriggerBy:    triggerBy,
		EnvOverrides: req.EnvOverrides,
		RequestID:    executionID,
	}

	// 异步执行工作流
	go func() {
		workflowEngine := engine.NewWorkflowEngine(rt.Ctx)
		_, _, err := workflowEngine.Execute(pipeline, event, triggerCtx)
		if err != nil {
			logger.Errorf("async workflow execute error: pipeline_id=%d execution_id=%s err=%v",
				pipeline.ID, executionID, err)
		}
	}()

	return executionID, nil
}

// triggerEventPipelineByService Service 调用触发工作流执行
func (rt *Router) triggerEventPipelineByService(c *gin.Context) {
	pipelineID := ginx.UrlParamInt64(c, "id")
	var f EventPipelineRequest
	ginx.BindJSON(c, &f)

	// 获取 Pipeline
	pipeline, err := models.GetEventPipeline(rt.Ctx, pipelineID)
	if err != nil {
		ginx.Bomb(http.StatusNotFound, "pipeline not found: %v", err)
	}

	executionID, err := rt.executePipelineTrigger(pipeline, &f, f.Username)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, "%v", err)
	}

	ginx.NewRender(c).Data(gin.H{
		"execution_id": executionID,
		"message":      "workflow execution started",
	}, nil)
}

// triggerEventPipelineByAPI API 触发工作流执行
func (rt *Router) triggerEventPipelineByAPI(c *gin.Context) {
	pipelineID := ginx.UrlParamInt64(c, "id")
	var f EventPipelineRequest
	ginx.BindJSON(c, &f)

	// 获取 Pipeline
	pipeline, err := models.GetEventPipeline(rt.Ctx, pipelineID)
	if err != nil {
		ginx.Bomb(http.StatusNotFound, "pipeline not found: %v", err)
	}

	// 检查权限
	me := c.MustGet("user").(*models.User)
	ginx.Dangerous(me.CheckGroupPermission(rt.Ctx, pipeline.TeamIds))

	executionID, err := rt.executePipelineTrigger(pipeline, &f, me.Username)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	ginx.NewRender(c).Data(gin.H{
		"execution_id": executionID,
		"message":      "workflow execution started",
	}, nil)
}

// ========== 执行记录 API ==========

// 获取所有Pipeline执行记录列表
func (rt *Router) listAllEventPipelineExecutions(c *gin.Context) {
	pipelineName := ginx.QueryStr(c, "pipeline_name", "")
	mode := ginx.QueryStr(c, "mode", "")
	status := ginx.QueryStr(c, "status", "")
	limit := ginx.QueryInt(c, "limit", 20)
	offset := ginx.QueryInt(c, "p", 1)

	if limit <= 0 || limit > 1000 {
		limit = 20
	}
	if offset <= 0 {
		offset = 1
	}

	executions, total, err := models.ListAllEventPipelineExecutions(rt.Ctx, pipelineName, mode, status, limit, (offset-1)*limit)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"list":  executions,
		"total": total,
	}, nil)
}

// 获取Pipeline执行记录列表
func (rt *Router) listEventPipelineExecutions(c *gin.Context) {
	pipelineID := ginx.UrlParamInt64(c, "id")
	mode := ginx.QueryStr(c, "mode", "")
	status := ginx.QueryStr(c, "status", "")
	limit := ginx.QueryInt(c, "limit", 20)
	offset := ginx.QueryInt(c, "p", 1)

	if limit <= 0 || limit > 1000 {
		limit = 20
	}
	if offset <= 0 {
		offset = 1
	}

	executions, total, err := models.ListEventPipelineExecutions(rt.Ctx, pipelineID, mode, status, limit, (offset-1)*limit)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"list":  executions,
		"total": total,
	}, nil)
}

// 获取单条执行记录详情
func (rt *Router) getEventPipelineExecution(c *gin.Context) {
	execID := ginx.UrlParamStr(c, "exec_id")

	detail, err := models.GetEventPipelineExecutionDetail(rt.Ctx, execID)
	if err != nil {
		ginx.Bomb(http.StatusNotFound, "execution not found: %v", err)
	}

	ginx.NewRender(c).Data(detail, nil)
}

// 获取Pipeline执行统计
func (rt *Router) getEventPipelineExecutionStats(c *gin.Context) {
	pipelineID := ginx.UrlParamInt64(c, "id")

	stats, err := models.GetEventPipelineExecutionStatistics(rt.Ctx, pipelineID)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(stats, nil)
}

// 清理历史执行记录
func (rt *Router) cleanEventPipelineExecutions(c *gin.Context) {
	var f struct {
		BeforeDays int `json:"before_days"`
	}
	ginx.BindJSON(c, &f)

	if f.BeforeDays <= 0 {
		f.BeforeDays = 30
	}

	beforeTime := time.Now().AddDate(0, 0, -f.BeforeDays).Unix()
	affected, err := models.DeleteEventPipelineExecutions(rt.Ctx, beforeTime)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"deleted": affected,
	}, nil)
}

// ========== SSE 流式执行接口 ==========

// streamEventPipeline SSE 流式执行工作流
func (rt *Router) streamEventPipeline(c *gin.Context) {
	pipelineID := ginx.UrlParamInt64(c, "id")

	var f EventPipelineRequest
	ginx.BindJSON(c, &f)

	pipeline, err := models.GetEventPipeline(rt.Ctx, pipelineID)
	if err != nil {
		ginx.Bomb(http.StatusNotFound, "pipeline not found: %v", err)
	}

	me := c.MustGet("user").(*models.User)
	ginx.Dangerous(me.CheckGroupPermission(rt.Ctx, pipeline.TeamIds))

	var event *models.AlertCurEvent
	if f.Event != nil {
		event = f.Event
	} else {
		event = &models.AlertCurEvent{
			TriggerTime: time.Now().Unix(),
		}
	}

	triggerCtx := &models.WorkflowTriggerContext{
		Mode:         models.TriggerModeAPI,
		TriggerBy:    me.Username,
		EnvOverrides: f.EnvOverrides,
		RequestID:    uuid.New().String(),
		Stream:       true, // 流式端点强制启用流式输出
	}

	workflowEngine := engine.NewWorkflowEngine(rt.Ctx)
	_, result, err := workflowEngine.Execute(pipeline, event, triggerCtx)
	if err != nil {
		ginx.Bomb(http.StatusInternalServerError, "execute failed: %v", err)
	}

	if result.Stream && result.StreamChan != nil {
		rt.handleStreamResponse(c, result, triggerCtx.RequestID)
		return
	}

	ginx.NewRender(c).Data(result, nil)
}

// handleStreamResponse 处理 SSE 流式响应
func (rt *Router) handleStreamResponse(c *gin.Context, result *models.WorkflowResult, requestID string) {
	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁用 nginx 缓冲
	c.Header("X-Request-ID", requestID)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		ginx.Bomb(http.StatusInternalServerError, "streaming not supported")
		return
	}

	// 发送初始连接成功消息
	initData := fmt.Sprintf(`{"type":"connected","request_id":"%s","timestamp":%d}`, requestID, time.Now().UnixMilli())
	fmt.Fprintf(c.Writer, "data: %s\n\n", initData)
	flusher.Flush()

	// 从 channel 读取并发送 SSE
	for {
		select {
		case chunk, ok := <-result.StreamChan:
			if !ok {
				// channel 关闭，发送结束标记
				return
			}

			data, err := json.Marshal(chunk)
			if err != nil {
				logger.Errorf("stream: failed to marshal chunk: %v", err)
				continue
			}

			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			flusher.Flush()

			if chunk.Done {
				return
			}

		case <-c.Request.Context().Done():
			// 客户端断开连接
			logger.Infof("stream: client disconnected, request_id=%s", requestID)
			return
		}
	}
}

// streamEventPipelineByService Service 调用的 SSE 流式执行接口
func (rt *Router) streamEventPipelineByService(c *gin.Context) {
	pipelineID := ginx.UrlParamInt64(c, "id")

	var f EventPipelineRequest
	ginx.BindJSON(c, &f)

	// 获取 Pipeline
	pipeline, err := models.GetEventPipeline(rt.Ctx, pipelineID)
	if err != nil {
		ginx.Bomb(http.StatusNotFound, "pipeline not found: %v", err)
	}

	// 准备事件
	var event *models.AlertCurEvent
	if f.Event != nil {
		event = f.Event
	} else {
		event = &models.AlertCurEvent{
			TriggerTime: time.Now().Unix(),
		}
	}

	// 创建触发上下文（流式模式）
	triggerCtx := &models.WorkflowTriggerContext{
		Mode:         models.TriggerModeAPI,
		TriggerBy:    f.Username,
		EnvOverrides: f.EnvOverrides,
		RequestID:    uuid.New().String(),
		Stream:       true, // 流式端点强制启用流式输出
	}

	// 执行工作流
	workflowEngine := engine.NewWorkflowEngine(rt.Ctx)
	_, result, err := workflowEngine.Execute(pipeline, event, triggerCtx)
	if err != nil {
		ginx.Bomb(http.StatusInternalServerError, "execute failed: %v", err)
	}

	// 检查是否是流式输出
	if result.Stream && result.StreamChan != nil {
		rt.handleStreamResponse(c, result, triggerCtx.RequestID)
		return
	}

	// 非流式：普通 JSON 响应（如果 processor 不支持流式）
	ginx.NewRender(c).Data(result, nil)
}
