package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/chat"
	"github.com/ccfos/nightingale/v6/aiagent/llm"
	_ "github.com/ccfos/nightingale/v6/aiagent/tools" // register builtin tools
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dscache"
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
		ChatID:     uuid.New().String(),
		Title:      "New Chat",
		LastUpdate: time.Now().Unix(),
		PageFrom:   models.AssistantPageInfo{Page: req.Page, Param: req.Param},
		UserID:     me.Id,
		IsNew:      true,
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

	// Acquire per-chat Redis lock. Short TTL + background renewal (see
	// models.ChatLock) — a long agent run extends the lease while alive;
	// a crashed process lets the lock expire within ChatLockTTL.
	lock, err := models.AcquireChatLock(c, rt.Redis, req.ChatID)
	ginx.Dangerous(err)
	if lock == nil {
		ginx.Bomb(http.StatusConflict, "chat is busy, please wait for the current message to finish")
		return
	}

	// Under lock: allocate seq_id
	maxSeq, err := models.AssistantMessageMaxSeqID(rt.Ctx, req.ChatID)
	if err != nil {
		lock.Release(context.Background(), rt.Redis)
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
		lock.Release(context.Background(), rt.Redis)
		ginx.Dangerous(err)
		return
	}

	// Update chat: title on first message, clear is_new, update timestamp.
	// Truncate by rune count (not byte count) so multi-byte characters like
	// Chinese/Japanese aren't sliced mid-character — byte-level slicing would
	// leave half a code point and render as '��'.
	if seqID == 1 {
		title := req.Query.Content
		if runes := []rune(title); len(runes) > 50 {
			title = string(runes[:50]) + "..."
		}
		chat.Title = title
	}
	chat.IsNew = false
	chat.LastUpdate = time.Now().Unix()
	models.AssistantChatSet(rt.Ctx, *chat)

	// Prepare stream cache
	aiagent.GetStreamCache().Create(streamID)

	// Create cancelable context. The parent context must outlive the agent's
	// own ReAct budget (set later from llmCfg.ExtraConfig.TimeoutSeconds), so
	// give it generous headroom — 15 min covers worst-case multi-tool flows
	// while still bounding stuck conversations.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

	key := msgKey(req.ChatID, seqID)
	rt.msgStateManager.Set(key, &msg, cancel)

	// Capture X-Language before the gin.Context goes out of scope — the goroutine
	// outlives this handler. Used to pin the agent's natural-language output to
	// the UI language (see chat.LanguageDirective).
	lang := c.GetHeader("X-Language")

	go rt.processAssistantMessage(ctx, cancel, lock, &msg, streamID, key, me.Id, lang)

	ginx.NewRender(c).Data(gin.H{
		"chat_id": req.ChatID,
		"seq_id":  seqID,
	}, nil)
}

func (rt *Router) processAssistantMessage(parentCtx context.Context, parentCancel context.CancelFunc, lock *models.ChatLock, msg *models.AssistantMessage, streamID, stateKey string, userId int64, lang string) {
	// Shutdown sequence (defer runs LIFO — reverse of registration order):
	//   1. parentCancel()           — signals watchdog to stop
	//   2. <-keepAliveDone          — wait for watchdog goroutine to fully exit
	//   3. lock.Release(...)        — CAS-delete the lock (background ctx so it
	//                                 still runs even though parentCtx is done)
	// This ordering prevents a late renew() from racing with the release and
	// logging a spurious "lost ownership" after we deleted the key ourselves.
	keepAliveDone := make(chan struct{})
	defer lock.Release(context.Background(), rt.Redis)
	defer func() { <-keepAliveDone }()
	defer parentCancel()

	// Watchdog: renews TTL every ChatLockRenewInterval until parentCtx is
	// canceled (success, error, timeout, or user cancel).
	go func() {
		defer close(keepAliveDone)
		lock.KeepAlive(parentCtx, rt.Redis)
	}()

	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[Assistant] PANIC: %v", r)
			rt.finishMessage(stateKey, streamID, msg, 500, fmt.Sprintf("internal error: %v", r))
		}
	}()

	// Gate: if the startup goroutine hasn't finished the first DB→FS skill sync
	// yet, wait for it here before InitSkills reads the registry off disk.
	// sync.Once makes this a cheap no-op after the first successful pass.
	rt.ensureAISkillsSynced()

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
	// llmCallTimeout caps a single LLM HTTP call (per-iteration). agentTotalTimeout
	// caps the entire ReAct loop across all tool calls. Multi-turn ReAct flows
	// (e.g. dashboard creation: list_busi_groups → list_datasources → list_files
	// → read_file → create_dashboard) easily run 7+ iterations, so the agent
	// budget must be several times the per-call budget.
	llmCallTimeout := 120000
	if extraConfig.TimeoutSeconds > 0 {
		llmCallTimeout = extraConfig.TimeoutSeconds * 1000
	}
	agentTotalTimeout := llmCallTimeout * 5
	if agentTotalTimeout < 5*60*1000 {
		agentTotalTimeout = 5 * 60 * 1000
	}

	llmClient, err := rt.llmClientCache.GetOrCreate(&llm.Config{
		Provider:      llmCfg.APIType,
		BaseURL:       llmCfg.APIURL,
		Model:         llmCfg.Model,
		APIKey:        llmCfg.APIKey,
		Headers:       extraConfig.CustomHeaders,
		Timeout:       llmCallTimeout,
		SkipSSLVerify: extraConfig.SkipTLSVerify,
		Proxy:         extraConfig.Proxy,
		Temperature:   extraConfig.Temperature,
		MaxTokens:     extraConfig.MaxTokens,
	})
	if err != nil {
		rt.finishMessage(stateKey, streamID, msg, 500, fmt.Sprintf("failed to create LLM client: %v", err))
		return
	}

	// ③ Resolve action key in priority order:
	//   1. Creation fast-path — if the user input has unambiguous creation
	//      intent (verb + resource noun, no query anti-verb), route directly
	//      to the creation action. Skips the LLM classifier entirely, which
	//      both saves latency and avoids the 15s classifier timeout that has
	//      been silently falling back to general_chat (see WARNING.log:
	//      "intent inference failed: context deadline exceeded").
	//   2. First message of a new chat with an explicit frontend key.
	//   3. LLM intent inference (30s budget — 15s was too tight in practice).
	var actionKey string
	switch {
	case chat.HasCreationIntent(msg.Query.Content):
		actionKey = string(models.ActionKeyCreation)
	case msg.SeqID == 1 && msg.Query.Action.Key != "":
		actionKey = string(msg.Query.Action.Key)
	default:
		inferCtx, inferCancel := context.WithTimeout(parentCtx, 30*time.Second)
		actionKey = chat.InferAction(inferCtx, llmClient, msg.Query.Content, history)
		inferCancel()
	}

	handler, ok := chat.Lookup(actionKey)
	if !ok {
		// Unknown key from frontend — fall back to general_chat
		actionKey = string(models.ActionKeyGeneralChat)
		handler, _ = chat.Lookup(actionKey)
	}

	logger.Infof("[Assistant] chat=%s seq=%d action_key=%s front_key=%q content=%q",
		msg.ChatID, msg.SeqID, actionKey, msg.Query.Action.Key, msg.Query.Content)

	// Build AIChatRequest for reusing existing action logic
	chatReq := &chat.AIChatRequest{
		ActionKey: actionKey,
		UserInput: msg.Query.Content,
		Context:   make(map[string]interface{}),
		Language:  lang,
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
	if ap.BusiGroupID > 0 {
		chatReq.Context["busi_group_id"] = ap.BusiGroupID
	}
	if len(ap.TeamIDs) > 0 {
		chatReq.Context["team_ids"] = ap.TeamIDs
	}

	// ④ Validate — on failure, silently fall back to general_chat instead of returning error
	if handler.Validate != nil {
		if err := handler.Validate(chatReq); err != nil {
			logger.Infof("[Assistant] validate failed for action_key=%s: %v, falling back to general_chat", actionKey, err)
			actionKey = string(models.ActionKeyGeneralChat)
			handler, _ = chat.Lookup(actionKey)
			chatReq.ActionKey = actionKey
			chatReq.Context = make(map[string]interface{})
		}
	}

	// ⑤ Preflight — hard gate. May halt the turn and emit structured responses
	// (e.g. ask the user to pick a busi group before a creation skill runs).
	toolDeps := &aiagent.ToolDeps{
		DBCtx:             rt.Ctx,
		GetPromClient:     func(dsId int64) prom.API { return rt.PromClients.GetCli(dsId) },
		GetSQLDatasource:  func(dsType string, dsId int64) (datasource.Datasource, bool) { return dscache.DsCache.Get(dsType, dsId) },
		FilterDatasources: rt.DatasourceCache.DatasourceFilter,
	}

	if handler.Preflight != nil {
		user, uerr := models.UserGetById(rt.Ctx, userId)
		if uerr != nil || user == nil {
			rt.finishMessage(stateKey, streamID, msg, 500, "failed to resolve user for preflight")
			return
		}
		halt, preResps, perr := handler.Preflight(parentCtx, toolDeps, chatReq, user)
		if perr != nil {
			logger.Warningf("[Assistant] preflight error for action_key=%s: %v", actionKey, perr)
		}
		if halt {
			rt.finishHaltedMessage(stateKey, streamID, msg, history, preResps)
			return
		}
	}

	// Select tools
	var tools []aiagent.AgentTool
	if handler.SelectTools != nil {
		toolNames := handler.SelectTools(chatReq)
		if toolNames != nil {
			tools = aiagent.GetBuiltinToolDefs(toolNames)
		}
	}

	userPrompt := ""
	if handler.BuildPrompt != nil {
		userPrompt = handler.BuildPrompt(chatReq)
	}
	// Pin LLM output to the UI language. Appended AFTER the action-specific
	// prompt so it lands at the tail of the agent's system instruction — the
	// position LLMs weight highest for "respond in X" directives. Empty lang
	// returns "" so we fall back to the LLM's auto-detection behavior.
	userPrompt += chat.LanguageDirective(lang)

	// Default inputs always carry user_input so downstream consumers
	// (skill autoselect's buildTaskContext, logging, tool params) can rely on
	// it. BuildInputs returns only the action-specific extras — merge them
	// on top rather than replacing the whole map, so handlers don't have to
	// remember to re-include the defaults.
	inputs := map[string]string{"user_input": msg.Query.Content}
	if handler.BuildInputs != nil {
		for k, v := range handler.BuildInputs(chatReq) {
			inputs[k] = v
		}
	}

	// Inject user_id for permission-aware builtin tools
	inputs["user_id"] = fmt.Sprintf("%d", userId)

	// 用 UserPromptRendered 而非 UserPromptTemplate：handler.BuildPrompt 已经用
	// fmt.Sprintf 把 msg.Query.Content 原样拼进 userPrompt，不能再经 text/template
	// 解析——否则用户问 "告警模板怎么写 {{ .Alertname }}" 会让 Parse 失败，整轮 500。
	//
	// Skills / MCP 绑定：agent.SkillIds/MCPServerIds 非空时走"精确注入"路径
	// （SkillNames + 固定 MCP server 列表），空则保留历史 AutoSelect 行为。详见
	// buildSkillConfigForAgent / buildMCPConfigForAgent 的注释。
	agentRunner := aiagent.NewAgent(&aiagent.AgentConfig{
		AgentMode:          aiagent.AgentModeReAct,
		Tools:              tools,
		Timeout:            agentTotalTimeout,
		Stream:             true,
		UserPromptRendered: userPrompt,
		Skills:             rt.buildSkillConfigForAgent(agent),
		MCP:                rt.buildMCPConfigForAgent(agent),
	}, aiagent.WithLLMClient(llmClient), aiagent.WithToolDeps(toolDeps))

	// Wire up the skill subsystem so SKILL.md content actually reaches the LLM
	// system prompt. InitSkills writes the resolved path into toolDeps.SkillsPath,
	// which list_files / read_file / grep_files use as the resolveBasePath
	// security anchor — without it those tools error with "skills path not configured".
	if skillsPath := rt.Center.AIAgent.SkillsPath; skillsPath != "" {
		agentRunner.InitSkills(skillsPath)
	}

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
	var createdAlertRules []string
	var createdDashboards []string
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
			// Capture successful tool results that have their own structured
			// UI cards (alert_rule, dashboard, ...) so the frontend can render
			// them outside of the plain markdown final answer.
			if chunk.Metadata != nil {
				toolName, _ := chunk.Metadata["tool"].(string)
				obs := strings.TrimSpace(chunk.Content)
				if obs != "" && !strings.HasPrefix(obs, "Error:") {
					switch toolName {
					case "create_alert_rule":
						createdAlertRules = append(createdAlertRules, obs)
					case "create_dashboard":
						createdDashboards = append(createdDashboards, obs)
					}
				}
			}
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
	if handler.ParseResponse != nil {
		responses = handler.ParseResponse(fullContent)
	}
	if len(responses) == 0 {
		responses = []models.AssistantMessageResponse{
			{ContentType: models.ContentTypeMarkdown, Content: fullContent, StreamID: streamID, IsFinish: true, IsFromAI: true},
		}
	} else {
		// Attach streamID to the first element for frontend stream matching
		responses[0].StreamID = streamID
	}

	// Append structured alert_rule cards for each successful create_alert_rule invocation.
	for _, ruleJSON := range createdAlertRules {
		responses = append(responses, models.AssistantMessageResponse{
			ContentType: models.ContentTypeAlertRule,
			Content:     ruleJSON,
			IsFinish:    true,
			IsFromAI:    true,
		})
	}
	// Same for dashboard cards.
	for _, dashJSON := range createdDashboards {
		responses = append(responses, models.AssistantMessageResponse{
			ContentType: models.ContentTypeDashboard,
			Content:     dashJSON,
			IsFinish:    true,
			IsFromAI:    true,
		})
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

// finishHaltedMessage ends the turn without running the agent (used by preflight
// hooks that ask the user for missing context). Responses are attached as normal
// success-path responses, streamID is wired to the first one, and the chat
// history records only the user's input.
func (rt *Router) finishHaltedMessage(stateKey, streamID string, msg *models.AssistantMessage, history []aiagent.ChatMessage, responses []models.AssistantMessageResponse) {
	aiagent.GetStreamCache().Finish(streamID)

	if len(responses) > 0 {
		responses[0].StreamID = streamID
		for i := range responses {
			responses[i].IsFinish = true
			responses[i].IsFromAI = true
		}
	}
	msg.Response = responses
	msg.IsFinish = true
	msg.CurStep = ""

	// Persist history with just the user's turn (no assistant content, since no agent ran).
	newHistory := append(history, aiagent.ChatMessage{Role: "user", Content: msg.Query.Content})
	msg.Extra.HistoryMessages, _ = json.Marshal(newHistory)

	if err := models.AssistantMessageSet(rt.Ctx, *msg); err != nil {
		logger.Errorf("[Assistant] failed to save halted message: %v", err)
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

	// SSE responses can outlive http.Server.WriteTimeout (40s by default), which
	// would otherwise close the underlying TCP connection mid-stream. Clear the
	// per-connection write deadline for this handler only.
	if rc := http.NewResponseController(c.Writer); rc != nil {
		_ = rc.SetWriteDeadline(time.Time{})
	}

	// Tie the reader's lifetime to the HTTP request so a client disconnect
	// (or normal handler return) releases the StreamCache forwarding goroutine
	// instead of leaking it until Finish/cleanup.
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	ch := aiagent.GetStreamCache().Read(ctx, req.StreamID)

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
