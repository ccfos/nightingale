package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
)

// ==================== MessageStateManager ====================

// MessageStateManager manages in-flight assistant messages.
type MessageStateManager struct {
	mu      sync.RWMutex
	states  map[string]*models.AssistantMessage // key: "chatID:seqID"
	cancels map[string]context.CancelFunc
}

func NewMessageStateManager() *MessageStateManager {
	return &MessageStateManager{
		states:  make(map[string]*models.AssistantMessage),
		cancels: make(map[string]context.CancelFunc),
	}
}

func msgKey(chatID string, seqID int64) string {
	return fmt.Sprintf("%s:%d", chatID, seqID)
}

func (m *MessageStateManager) Set(key string, msg *models.AssistantMessage, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[key] = msg
	if cancel != nil {
		m.cancels[key] = cancel
	}
}

func (m *MessageStateManager) Get(key string) (*models.AssistantMessage, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.states[key]
	return v, ok
}

func (m *MessageStateManager) Remove(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, key)
	delete(m.cancels, key)
}

func (m *MessageStateManager) Cancel(key string) bool {
	m.mu.RLock()
	cancel, ok := m.cancels[key]
	m.mu.RUnlock()
	if ok && cancel != nil {
		cancel()
		return true
	}
	return false
}

func (m *MessageStateManager) UpdateMsg(key string, fn func(msg *models.AssistantMessage)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if msg, ok := m.states[key]; ok {
		fn(msg)
	}
}

func (m *MessageStateManager) GetStreamID(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if msg, ok := m.states[key]; ok && len(msg.Response) > 0 {
		return msg.Response[0].StreamID
	}
	return ""
}

// ==================== Chat Handlers ====================

func (rt *Router) assistantChatNew(c *gin.Context) {
	me := c.MustGet("user").(*models.User)

	var req struct {
		Page  models.AssistantPageType `json:"page"`
		Param json.RawMessage          `json:"param"`
	}
	ginx.BindJSON(c, &req)

	var param models.AssistantPageInfoParam
	if len(req.Param) > 0 {
		json.Unmarshal(req.Param, &param)
	}

	chat := models.AssistantChat{
		ChatID:          uuid.New().String(),
		Title:           "New Chat",
		LastUpdate:      time.Now().Unix(),
		PageFrom:        models.AssistantPageInfo{Page: req.Page, Param: param},
		RecommendAction: []models.AssistantAction{},
		UserID:          me.Id,
		IsNew:           true,
	}

	ginx.Dangerous(models.AssistantChatSet(rt.Ctx, chat))
	ginx.NewRender(c).Data(chat, nil)
}

func (rt *Router) assistantChatHistory(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	chats, err := models.AssistantChatGetsByUserID(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	if chats == nil {
		chats = []models.AssistantChat{}
	}
	ginx.NewRender(c).Data(chats, nil)
}

func (rt *Router) assistantChatDel(c *gin.Context) {
	chatID := c.Param("chatId")
	me := c.MustGet("user").(*models.User)
	_, err := models.AssistantChatCheckOwner(rt.Ctx, chatID, me.Id)
	ginx.Dangerous(err)
	ginx.NewRender(c).Message(models.AssistantChatDelete(rt.Ctx, chatID))
}

// ==================== Message Handlers ====================

func inferActionKey(pageFrom models.AssistantPageInfo) models.AssistantActionKey {
	switch pageFrom.Page {
	default:
		return models.ActionKeyQueryGenerator
	}
}

func (rt *Router) assistantMessageNew(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	var req models.AssistantSendRequest
	ginx.BindJSON(c, &req)

	if req.ChatID == "" {
		ginx.Bomb(http.StatusBadRequest, "chat_id is required")
		return
	}
	if req.Query.Content == "" {
		ginx.Bomb(http.StatusBadRequest, "query.content is required")
		return
	}

	// Verify chat ownership
	chat, err := models.AssistantChatCheckOwner(rt.Ctx, req.ChatID, me.Id)
	ginx.Dangerous(err)

	// Acquire per-chat Redis lock
	chatLockKey := models.AssistantChatLockKey(req.ChatID)
	locked, err := rt.Redis.SetNX(c, chatLockKey, "1", 5*time.Minute).Result()
	ginx.Dangerous(err)
	if !locked {
		ginx.Bomb(http.StatusConflict, "chat is busy, please wait for the current message to finish")
		return
	}

	// Under lock: allocate seq_id
	maxSeq, err := models.AssistantMessageMaxSeqID(rt.Ctx, req.ChatID)
	if err != nil {
		rt.Redis.Del(c, chatLockKey)
		ginx.Dangerous(err)
		return
	}
	seqID := maxSeq + 1

	// Build message
	streamID := uuid.New().String()
	msg := models.AssistantMessage{
		ChatID: req.ChatID,
		SeqID:  seqID,
		Query:  req.Query,
		Response: []models.AssistantMessageResponse{
			{ContentType: models.ContentTypeMarkdown, StreamID: streamID, IsFromAI: true},
		},
		RecommendAction: []models.AssistantAction{},
	}

	// Persist initial message
	if err := models.AssistantMessageSet(rt.Ctx, msg); err != nil {
		rt.Redis.Del(c, chatLockKey)
		ginx.Dangerous(err)
		return
	}

	// Update chat: title on first message, clear is_new, update timestamp
	if seqID == 1 {
		title := req.Query.Content
		if len(title) > 50 {
			title = title[:50] + "..."
		}
		chat.Title = title
	}
	chat.IsNew = false
	chat.LastUpdate = time.Now().Unix()
	models.AssistantChatSet(rt.Ctx, *chat)

	// Prepare stream cache
	aiagent.GetStreamCache().Create(streamID)

	// Create cancelable context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	key := msgKey(req.ChatID, seqID)
	rt.msgStateManager.Set(key, &msg, cancel)

	go rt.processAssistantMessage(ctx, cancel, chatLockKey, &msg, streamID, key)

	ginx.NewRender(c).Data(gin.H{
		"chat_id": req.ChatID,
		"seq_id":  seqID,
	}, nil)
}

func (rt *Router) processAssistantMessage(parentCtx context.Context, parentCancel context.CancelFunc, chatLockKey string, msg *models.AssistantMessage, streamID, stateKey string) {
	defer parentCancel()
	defer rt.Redis.Del(context.Background(), chatLockKey)

	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[Assistant] PANIC: %v", r)
			rt.finishMessage(stateKey, streamID, msg, 500, fmt.Sprintf("internal error: %v", r))
		}
	}()

	streamCache := aiagent.GetStreamCache()

	// Resolve action key
	actionKey := string(msg.Query.Action.Key)
	if actionKey == "" {
		actionKey = string(inferActionKey(msg.Query.PageFrom))
	}

	handler, ok := actionRegistry[actionKey]
	if !ok {
		rt.finishMessage(stateKey, streamID, msg, 400, fmt.Sprintf("unsupported action_key: %s", actionKey))
		return
	}

	// Build AIChatRequest for reusing existing action logic
	chatReq := &AIChatRequest{
		ActionKey: actionKey,
		UserInput: msg.Query.Content,
		Context:   make(map[string]interface{}),
	}

	// Extract action.param into context
	ap := msg.Query.Action.Param
	if ap.DatasourceType != "" {
		chatReq.Context["datasource_type"] = ap.DatasourceType
	}
	if ap.DatasourceID > 0 {
		chatReq.Context["datasource_id"] = ap.DatasourceID
	}
	if ap.DatabaseName != "" {
		chatReq.Context["database_name"] = ap.DatabaseName
	}
	if ap.TableName != "" {
		chatReq.Context["table_name"] = ap.TableName
	}

	// Validate
	if handler.validate != nil {
		if err := handler.validate(chatReq); err != nil {
			rt.finishMessage(stateKey, streamID, msg, 400, err.Error())
			return
		}
	}

	// Find AI agent
	agent, err := models.AIAgentGetByUseCase(rt.Ctx, handler.useCase)
	if err != nil || agent == nil {
		rt.finishMessage(stateKey, streamID, msg, 400, fmt.Sprintf("no AI agent configured for use_case=%s", handler.useCase))
		return
	}

	llmCfg, err := models.AILLMConfigGetById(rt.Ctx, agent.LLMConfigId)
	if err != nil || llmCfg == nil {
		rt.finishMessage(stateKey, streamID, msg, 400, "referenced LLM config not found")
		return
	}

	extraConfig := llmCfg.ExtraConfig
	timeout := 120000
	if extraConfig.TimeoutSeconds > 0 {
		timeout = extraConfig.TimeoutSeconds * 1000
	}

	// Select tools
	var tools []aiagent.AgentTool
	if handler.selectTools != nil {
		toolNames := handler.selectTools(chatReq)
		if toolNames != nil {
			tools = aiagent.GetBuiltinToolDefs(toolNames)
		}
	}

	userPrompt := ""
	if handler.buildPrompt != nil {
		userPrompt = handler.buildPrompt(chatReq)
	}

	inputs := map[string]string{"user_input": msg.Query.Content}
	if handler.buildInputs != nil {
		inputs = handler.buildInputs(chatReq)
	}

	// Load multi-turn history from previous message's Extra
	var history []aiagent.ChatMessage
	if msg.SeqID > 1 {
		prevMsg, _ := models.AssistantMessageGet(rt.Ctx, msg.ChatID, msg.SeqID-1)
		if prevMsg != nil && len(prevMsg.Extra.HistoryMessages) > 0 {
			json.Unmarshal(prevMsg.Extra.HistoryMessages, &history)
		}
	}

	agentCfg := aiagent.NewAgent(&aiagent.AIAgentConfig{
		Provider:           llmCfg.APIType,
		LLMURL:             llmCfg.APIURL,
		Model:              llmCfg.Model,
		APIKey:             llmCfg.APIKey,
		Headers:            extraConfig.CustomHeaders,
		AgentMode:          aiagent.AgentModeReAct,
		Tools:              tools,
		Timeout:            timeout,
		Stream:             true,
		UserPromptTemplate: userPrompt,
		SkipSSLVerify:      extraConfig.SkipTLSVerify,
		Proxy:              extraConfig.Proxy,
		Temperature:        extraConfig.Temperature,
		MaxTokens:          extraConfig.MaxTokens,
		History:            history,
	})

	aiagent.SetPromClientGetter(func(dsId int64) prom.API {
		return rt.PromClients.GetCli(dsId)
	})

	wfStreamChan := make(chan *models.StreamChunk, 100)
	wfCtx := &models.WorkflowContext{
		Stream:     true,
		StreamChan: wfStreamChan,
		Inputs:     inputs,
		ParentCtx:  parentCtx,
	}

	_, _, processErr := agentCfg.Process(rt.Ctx, wfCtx)
	if processErr != nil {
		logger.Errorf("[Assistant] Process error: %v", processErr)
	}

	// Consume stream chunks
	var fullContent string
	executedTools := false
	for chunk := range wfCtx.StreamChan {
		select {
		case <-parentCtx.Done():
			rt.finishMessage(stateKey, streamID, msg, -2, "cancelled")
			return
		default:
		}

		switch chunk.Type {
		case models.StreamTypeThinking:
			delta := chunk.Delta
			if delta == "" {
				delta = chunk.Content
			}
			if delta != "" {
				streamCache.AddReason(streamID, delta)
			}
		case models.StreamTypeText:
			delta := chunk.Delta
			if delta == "" {
				delta = chunk.Content
			}
			if delta != "" {
				streamCache.AddReason(streamID, delta)
			}
		case models.StreamTypeToolCall:
			executedTools = true
			step := "Using tools..."
			if chunk.Content != "" {
				step = chunk.Content
			}
			rt.msgStateManager.UpdateMsg(stateKey, func(m *models.AssistantMessage) {
				m.CurStep = step
				m.ExecutedTools = true
			})
		case models.StreamTypeToolResult:
			rt.msgStateManager.UpdateMsg(stateKey, func(m *models.AssistantMessage) {
				m.CurStep = "Processing tool result..."
			})
		case models.StreamTypeError:
			errMsg := chunk.Error
			if errMsg == "" {
				errMsg = chunk.Content
			}
			rt.finishMessage(stateKey, streamID, msg, 500, errMsg)
			return
		case models.StreamTypeDone:
			if chunk.Content != "" {
				fullContent = chunk.Content
				streamCache.AddContent(streamID, chunk.Content)
			}
		}
	}

	streamCache.Finish(streamID)

	// Build final response
	msg.Response = []models.AssistantMessageResponse{
		{ContentType: models.ContentTypeMarkdown, Content: fullContent, StreamID: streamID, IsFinish: true, IsFromAI: true},
	}
	msg.IsFinish = true
	msg.CurStep = ""
	msg.ExecutedTools = executedTools

	// Persist history
	newHistory := append(history, aiagent.ChatMessage{Role: "user", Content: msg.Query.Content})
	newHistory = append(newHistory, aiagent.ChatMessage{Role: "assistant", Content: fullContent})
	msg.Extra.HistoryMessages, _ = json.Marshal(newHistory)

	// Save to DB (UPSERT)
	if err := models.AssistantMessageSet(rt.Ctx, *msg); err != nil {
		logger.Errorf("[Assistant] failed to save message: %v", err)
	}

	rt.msgStateManager.Remove(stateKey)
}

func (rt *Router) finishMessage(stateKey, streamID string, msg *models.AssistantMessage, errCode int, errMsg string) {
	aiagent.GetStreamCache().Finish(streamID)

	msg.IsFinish = true
	msg.ErrCode = errCode
	msg.ErrMsg = errMsg

	if err := models.AssistantMessageSet(rt.Ctx, *msg); err != nil {
		logger.Errorf("[Assistant] failed to save error message: %v", err)
	}

	rt.msgStateManager.Remove(stateKey)
}

// ==================== Detail / History / Cancel ====================

func (rt *Router) assistantMessageDetail(c *gin.Context) {
	var req struct {
		ChatID string `json:"chat_id"`
		SeqID  int64  `json:"seq_id"`
	}
	ginx.BindJSON(c, &req)

	me := c.MustGet("user").(*models.User)
	_, err := models.AssistantChatCheckOwner(rt.Ctx, req.ChatID, me.Id)
	ginx.Dangerous(err)

	key := msgKey(req.ChatID, req.SeqID)
	if msg, ok := rt.msgStateManager.Get(key); ok {
		ginx.NewRender(c).Data(msg, nil)
		return
	}

	msg, err := models.AssistantMessageGet(rt.Ctx, req.ChatID, req.SeqID)
	ginx.Dangerous(err)
	if msg == nil {
		ginx.Bomb(http.StatusNotFound, "message not found")
		return
	}
	ginx.NewRender(c).Data(msg, nil)
}

func (rt *Router) assistantMessageHistory(c *gin.Context) {
	var req struct {
		ChatID string `json:"chat_id"`
	}
	ginx.BindJSON(c, &req)

	if req.ChatID == "" {
		ginx.Bomb(http.StatusBadRequest, "chat_id is required")
		return
	}

	me := c.MustGet("user").(*models.User)
	_, err := models.AssistantChatCheckOwner(rt.Ctx, req.ChatID, me.Id)
	ginx.Dangerous(err)

	msgs, err := models.AssistantMessageGetsByChat(rt.Ctx, req.ChatID)
	ginx.Dangerous(err)
	if msgs == nil {
		msgs = []models.AssistantMessage{}
	}
	ginx.NewRender(c).Data(msgs, nil)
}

func (rt *Router) assistantMessageCancel(c *gin.Context) {
	var req struct {
		ChatID string `json:"chat_id"`
		SeqID  int64  `json:"seq_id"`
	}
	ginx.BindJSON(c, &req)

	me := c.MustGet("user").(*models.User)
	_, err := models.AssistantChatCheckOwner(rt.Ctx, req.ChatID, me.Id)
	ginx.Dangerous(err)

	key := msgKey(req.ChatID, req.SeqID)
	streamID := rt.msgStateManager.GetStreamID(key)

	if !rt.msgStateManager.Cancel(key) {
		ginx.Bomb(http.StatusNotFound, "message not executing or not found")
		return
	}

	if streamID != "" {
		aiagent.GetStreamCache().Finish(streamID)
	}

	// Persist cancel state via status column (like fc-model)
	models.AssistantMessageSetStatus(rt.Ctx, req.ChatID, req.SeqID, models.MessageStatusCancel)

	ginx.NewRender(c).Message(nil)
}

// ==================== Stream Handler ====================

func (rt *Router) assistantStream(c *gin.Context) {
	var req struct {
		StreamID string `json:"stream_id"`
	}
	ginx.BindJSON(c, &req)

	if req.StreamID == "" {
		ginx.Bomb(http.StatusBadRequest, "stream_id is required")
		return
	}

	ch := aiagent.GetStreamCache().Read(req.StreamID)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Stream(func(w io.Writer) bool {
		msg, ok := <-ch
		if !ok {
			fmt.Fprintf(w, "event: finish\ndata:\n\n")
			c.Writer.Flush()
			return false
		}
		data, _ := json.Marshal(msg)
		fmt.Fprintf(w, "data: %s\n\n", data)
		c.Writer.Flush()
		return true
	})
}
