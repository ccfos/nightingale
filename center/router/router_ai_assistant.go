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
	"github.com/ccfos/nightingale/v6/aiagent/llm"
	_ "github.com/ccfos/nightingale/v6/aiagent/tools" // register builtin tools
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
)

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

	chat := models.AssistantChat{
		ChatID:          uuid.New().String(),
		Title:           "New Chat",
		LastUpdate:      time.Now().Unix(),
		PageFrom:        models.AssistantPageInfo{Page: req.Page, Param: req.Param},
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

	go rt.processAssistantMessage(ctx, cancel, chatLockKey, &msg, streamID, key, me.Id)

	ginx.NewRender(c).Data(gin.H{
		"chat_id": req.ChatID,
		"seq_id":  seqID,
	}, nil)
}

func (rt *Router) processAssistantMessage(parentCtx context.Context, parentCancel context.CancelFunc, chatLockKey string, msg *models.AssistantMessage, streamID, stateKey string, userId int64) {
	defer parentCancel()
	defer rt.Redis.Del(context.Background(), chatLockKey)

	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[Assistant] PANIC: %v", r)
			rt.finishMessage(stateKey, streamID, msg, 500, fmt.Sprintf("internal error: %v", r))
		}
	}()

	streamCache := aiagent.GetStreamCache()

	// ① Load multi-turn history early (needed for LLM intent inference)
	var history []aiagent.ChatMessage
	if msg.SeqID > 1 {
		prevMsg, _ := models.AssistantMessageGet(rt.Ctx, msg.ChatID, msg.SeqID-1)
		if prevMsg != nil && len(prevMsg.Extra.HistoryMessages) > 0 {
			if err := json.Unmarshal(prevMsg.Extra.HistoryMessages, &history); err != nil {
				logger.Warningf("[Assistant] failed to unmarshal history for chat=%s seq=%d: %v", msg.ChatID, msg.SeqID-1, err)
			}
		}
	}

	// ② Create LLM client early (shared by intent inference and agent execution)
	// Always use "chat" useCase to find the agent config for the LLM client.
	agent, err := models.AIAgentGetByUseCase(rt.Ctx, "chat")
	if err != nil || agent == nil {
		rt.finishMessage(stateKey, streamID, msg, 400, "no AI agent configured for use_case=chat")
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

	llmClient, err := rt.llmClientCache.GetOrCreate(&llm.Config{
		Provider:      llmCfg.APIType,
		BaseURL:       llmCfg.APIURL,
		Model:         llmCfg.Model,
		APIKey:        llmCfg.APIKey,
		Headers:       extraConfig.CustomHeaders,
		Timeout:       timeout,
		SkipSSLVerify: extraConfig.SkipTLSVerify,
		Proxy:         extraConfig.Proxy,
		Temperature:   extraConfig.Temperature,
		MaxTokens:     extraConfig.MaxTokens,
	})
	if err != nil {
		rt.finishMessage(stateKey, streamID, msg, 500, fmt.Sprintf("failed to create LLM client: %v", err))
		return
	}

	// ③ Resolve action key:
	//   - First message of a new chat with an explicit key: use the frontend-provided key
	//   - Otherwise (key empty or subsequent messages): LLM intent inference
	var actionKey string
	if msg.SeqID == 1 && msg.Query.Action.Key != "" {
		actionKey = string(msg.Query.Action.Key)
	} else {
		inferCtx, inferCancel := context.WithTimeout(parentCtx, 15*time.Second)
		actionKey = inferActionKeyByLLM(inferCtx, llmClient, msg.Query.Content, history)
		inferCancel()
	}

	handler, ok := actionRegistry[actionKey]
	if !ok {
		// Unknown key from frontend — fall back to general_chat
		actionKey = string(models.ActionKeyGeneralChat)
		handler = actionRegistry[actionKey]
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

	// ④ Validate — on failure, silently fall back to general_chat instead of returning error
	if handler.validate != nil {
		if err := handler.validate(chatReq); err != nil {
			logger.Infof("[Assistant] validate failed for action_key=%s: %v, falling back to general_chat", actionKey, err)
			actionKey = string(models.ActionKeyGeneralChat)
			handler = actionRegistry[actionKey]
			chatReq.ActionKey = actionKey
			chatReq.Context = make(map[string]interface{})
		}
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

	// Inject user_id for permission-aware builtin tools
	inputs["user_id"] = fmt.Sprintf("%d", userId)

	agentRunner := aiagent.NewAgent(&aiagent.AgentConfig{
		AgentMode:          aiagent.AgentModeReAct,
		Tools:              tools,
		Timeout:            timeout,
		Stream:             true,
		UserPromptTemplate: userPrompt,
	}, aiagent.WithLLMClient(llmClient))

	aiagent.SetPromClientGetter(func(dsId int64) prom.API {
		return rt.PromClients.GetCli(dsId)
	})

	aiagent.SetDBCtx(rt.Ctx)
	aiagent.SetDatasourceFilter(rt.DatasourceCache.DatasourceFilter)

	streamChan := make(chan *aiagent.StreamChunk, 100)
	agentReq := &aiagent.AgentRequest{
		Params:     inputs,
		History:    history,
		StreamChan: streamChan,
		ParentCtx:  parentCtx,
	}

	_, processErr := agentRunner.Run(parentCtx, agentReq)
	if processErr != nil {
		logger.Errorf("[Assistant] Process error: %v", processErr)
	}

	// Consume stream chunks
	var fullContent string
	var fullReasoning string
	executedTools := false
	for chunk := range streamChan {
		select {
		case <-parentCtx.Done():
			rt.finishMessage(stateKey, streamID, msg, -2, "cancelled")
			return
		default:
		}

		switch chunk.Type {
		case aiagent.StreamTypeThinking:
			delta := chunk.Delta
			if delta == "" {
				delta = chunk.Content
			}
			if delta != "" {
				fullReasoning += delta
				streamCache.AddReason(streamID, delta)
			}
		case aiagent.StreamTypeText:
			delta := chunk.Delta
			if delta == "" {
				delta = chunk.Content
			}
			if delta != "" {
				fullReasoning += delta
				streamCache.AddReason(streamID, delta)
			}
		case aiagent.StreamTypeToolCall:
			executedTools = true
			step := "Using tools..."
			if chunk.Content != "" {
				step = chunk.Content
			}
			rt.msgStateManager.UpdateMsg(stateKey, func(m *models.AssistantMessage) {
				m.CurStep = step
				m.ExecutedTools = true
			})
		case aiagent.StreamTypeToolResult:
			rt.msgStateManager.UpdateMsg(stateKey, func(m *models.AssistantMessage) {
				m.CurStep = "Processing tool result..."
			})
		case aiagent.StreamTypeError:
			errMsg := chunk.Error
			if errMsg == "" {
				errMsg = chunk.Content
			}
			rt.finishMessage(stateKey, streamID, msg, 500, errMsg)
			return
		case aiagent.StreamTypeDone:
			if chunk.Content != "" {
				fullContent = chunk.Content
				streamCache.AddContent(streamID, chunk.Content)
			}
		}
	}

	streamCache.Finish(streamID)

	// Build final response: try action-specific parsing first, fall back to single markdown
	var responses []models.AssistantMessageResponse
	if handler.parseResponse != nil {
		responses = handler.parseResponse(fullContent)
	}
	if len(responses) == 0 {
		responses = []models.AssistantMessageResponse{
			{ContentType: models.ContentTypeMarkdown, Content: fullContent, StreamID: streamID, IsFinish: true, IsFromAI: true},
		}
	} else {
		// Attach streamID to the first element for frontend stream matching
		responses[0].StreamID = streamID
	}

	// Prepend reasoning as the first response item so it is persisted and
	// returned when loading historical conversations.
	if fullReasoning != "" {
		reasoningResp := models.AssistantMessageResponse{
			ContentType: models.ContentTypeReasoning,
			Content:     fullReasoning,
			IsFinish:    true,
			IsFromAI:    true,
		}
		responses = append([]models.AssistantMessageResponse{reasoningResp}, responses...)
	}

	msg.Response = responses
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

	models.AssistantMessageSetStatus(rt.Ctx, req.ChatID, req.SeqID, models.MessageStatusCancel)

	ginx.NewRender(c).Message(nil)
}

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

// serviceUser reads X-Service-Username header, resolves the user from DB,
// and injects it into the gin context. This allows v1 service routes to
// reuse the same handlers as the frontend.
func (rt *Router) serviceUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.GetHeader("X-Service-Username")
		if username == "" {
			ginx.Bomb(http.StatusBadRequest, "X-Service-Username header is required")
		}
		user, err := models.UserGetByUsername(rt.Ctx, username)
		if err != nil {
			ginx.Bomb(http.StatusInternalServerError, err.Error())
		}
		if user == nil {
			ginx.Bomb(http.StatusNotFound, "user not found: %s", username)
		}
		c.Set("user", user)
		c.Set("userid", user.Id)
		c.Set("username", user.Username)
		c.Next()
	}
}
