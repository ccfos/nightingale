package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
)

// MessageState 是 owner 实例本地的消息工作副本 + Redis 持久化包装。
// 多实例化改造后，"哪台实例在跑这条消息"由 ChatLock 决定，detail/cancel/stream
// 全靠 Redis 中的快照与 Stream，不再依赖进程内 map。
//
// 单一 owner 写入，无并发写竞态——由 ChatLock 保证；不加锁。/detail 等只读路径
// 直接从 Redis 读，不会触碰 s.msg 指针。
type MessageState struct {
	rds storage.Redis
	msg *models.AssistantMessage
}

func NewMessageState(rds storage.Redis, msg *models.AssistantMessage) *MessageState {
	return &MessageState{rds: rds, msg: msg}
}

// Update 修改本地副本并把整个 JSON 写回 Redis。Redis 写失败只 warn——本地状态
// 已变更，下一次 Update 会重试覆盖。
//
// Cancel race guard：/cancel handler 一旦把 MsgCancelKey 设上，它的 Redis 终态
// 就是权威——这里再写 MsgStateSet 会盖掉 cancelled 状态导致 /detail 显示成功
// 但 history 因 status=-2 过滤掉这条，前后矛盾。本地副本仍然 mutate，因为 owner
// goroutine 后续逻辑（如 m.ExecutedTools）依赖它，只是不再持久化到 Redis。
func (s *MessageState) Update(ctx context.Context, fn func(*models.AssistantMessage)) {
	fn(s.msg)
	if cancelled, _ := models.MsgCancelExists(ctx, s.rds, s.msg.ChatID, s.msg.SeqID); cancelled {
		return
	}
	if err := models.MsgStateSet(ctx, s.rds, s.msg); err != nil {
		logger.Warningf("[Assistant] persist msg state chat=%s seq=%d: %v", s.msg.ChatID, s.msg.SeqID, err)
	}
}

// Persist 显式刷一次（在直接修改 s.msg 字段之后调用）。同 Update 一样，
// 一旦 cancel marker 设上就停止写——避免覆盖 cancel handler 的权威终态。
func (s *MessageState) Persist(ctx context.Context) {
	if cancelled, _ := models.MsgCancelExists(ctx, s.rds, s.msg.ChatID, s.msg.SeqID); cancelled {
		return
	}
	if err := models.MsgStateSet(ctx, s.rds, s.msg); err != nil {
		logger.Warningf("[Assistant] persist msg state chat=%s seq=%d: %v", s.msg.ChatID, s.msg.SeqID, err)
	}
}

// Msg 返回本地副本指针，调用方必须只读；写请走 Update
func (s *MessageState) Msg() *models.AssistantMessage { return s.msg }

// parseChatIDFromStreamID 把 streamID 拆出 chatID。streamID 格式为
// "<chatID>:<seqID>:<uuid>"（旧格式 "<chatID>:<uuid>" 也兼容——chatID 都是首段）
// ——这样 /assistant/stream 接口在不改 wire format 的前提下，后端仍能从 streamID
// 定位到对应 Redis Stream key。
func parseChatIDFromStreamID(streamID string) string {
	if i := strings.Index(streamID, ":"); i > 0 {
		return streamID[:i]
	}
	return ""
}

// parseSeqIDFromStreamID 拆出 seqID。新格式 3 段返回真实 seqID；旧格式 2 段返回
// 0——调用方据此判断是否能做 MsgStateGet 级别的 orphan 检测，0 时回退到 chat 粒度
// 的 ChatLockHeld 老逻辑。
func parseSeqIDFromStreamID(streamID string) int64 {
	parts := strings.SplitN(streamID, ":", 3)
	if len(parts) < 3 {
		return 0
	}
	n, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0
	}
	return n
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

	// streamID 内嵌 chatID + seqID："<chatID>:<seqID>:<uuid>"——
	// /assistant/stream 接口收到 streamID 后能据此还原出 chatID（用来定位 Redis
	// Stream key 的 hash tag）和 seqID（用来 MsgStateGet 做 message 粒度的 orphan
	// 判定），不需要前端多传字段，wire format 完全不变。
	// 旧格式 "<chatID>:<uuid>" 由 parser 兼容（seqID 解析失败回 0，watchdog 自动
	// 回退到 ChatLockHeld 老逻辑）。
	streamID := fmt.Sprintf("%s:%d:%s", req.ChatID, seqID, uuid.New().String())
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

	// 写入 Redis 上的初始快照——detail 接口在 owner 还没开始流式输出之前来查就能
	// 直接从 Redis 拿到。
	state := NewMessageState(rt.Redis, &msg)
	state.Persist(c)

	// 同步种入 init marker，让 stream key 立刻存在。客户端拿到 streamID 后立即
	// 连 /assistant/stream 时，Exists 校验能立刻通过，不会因为 owner 还没首包
	// 而被误判成"无效 streamID"。
	//
	// Init 失败必须在返回 streamID 给前端之前处理掉：否则 stream key 不存在，
	// 前端连 /assistant/stream 后 Exists 返回 (false, nil)，handler 立即写
	// finish event 关闭，owner 后续 Append 全部丢失，UI 表现为「消息成功创建
	// 但永远收不到任何内容」。go-redis 自带连接级 retry，但偶发抖动仍可能让
	// Init 失败而后续 Append 成功（不同次调用、不同 backoff），所以这里再叠
	// 一层 app-level retry 吃瞬时抖动；仍失败就当 Redis 不可用，释放 lock、
	// 把刚 Persist 的占位消息打成 cancelled（避免幽灵行），让前端看到清晰失败。
	var initErr error
	for i := 0; i < 3; i++ {
		initErr = rt.streamBus.Init(c, msg.ChatID, streamID)
		if initErr == nil {
			break
		}
		logger.Warningf("[Assistant] streamBus.Init chat=%s stream=%s attempt=%d: %v", msg.ChatID, streamID, i+1, initErr)
		time.Sleep(50 * time.Millisecond)
	}
	if initErr != nil {
		models.AssistantMessageSetStatus(rt.Ctx, msg.ChatID, msg.SeqID, models.MessageStatusCancel)
		lock.Release(context.Background(), rt.Redis)
		ginx.Bomb(http.StatusInternalServerError, "stream init failed: %v", initErr)
		return
	}

	// Create cancelable context. The parent context must outlive the agent's
	// own ReAct budget (set later from llmCfg.ExtraConfig.TimeoutSeconds), so
	// give it generous headroom — 15 min covers worst-case multi-tool flows
	// while still bounding stuck conversations.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

	// Capture X-Language before the gin.Context goes out of scope — the goroutine
	// outlives this handler. Used to pin the agent's natural-language output to
	// the UI language (see chat.LanguageDirective).
	lang := c.GetHeader("X-Language")

	go rt.processAssistantMessage(ctx, cancel, lock, state, streamID, me.Id, lang)

	ginx.NewRender(c).Data(gin.H{
		"chat_id": req.ChatID,
		"seq_id":  seqID,
	}, nil)
}

func (rt *Router) processAssistantMessage(parentCtx context.Context, parentCancel context.CancelFunc, lock *models.ChatLock, state *MessageState, streamID string, userId int64, lang string) {
	msg := state.Msg()
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

	// Cancel 通道：任意实例的 /assistant/message/cancel 调用 PUBLISH 到这个频道，
	// 我们（owner）订阅并转换成本地 ctx 取消，让正在跑的 LLM/工具循环能立即停。
	// 同时兜底每 2s 检查 cancel 标志位（pubsub 偶发漏发时收尾）。
	cancelSub := rt.pubsubBus.Subscribe(parentCtx, models.MsgCancelChannel(msg.ChatID, msg.SeqID))
	go func() {
		defer cancelSub.Close()
		// 启动期同步检查一次：assistantMessageNew 返回客户端后客户端可能立即调
		// /cancel，那个 PUBLISH 在 Subscribe 注册前发出会被 Redis 丢掉，仅靠
		// ticker 兜底要等 2s。这里先看一眼 SET 标志位，把盲区压到 0。
		if exists, _ := models.MsgCancelExists(parentCtx, rt.Redis, msg.ChatID, msg.SeqID); exists {
			parentCancel()
			return
		}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-parentCtx.Done():
				return
			case <-cancelSub.Channel():
				parentCancel()
				return
			case <-ticker.C:
				if exists, _ := models.MsgCancelExists(parentCtx, rt.Redis, msg.ChatID, msg.SeqID); exists {
					parentCancel()
					return
				}
			}
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[Assistant] PANIC: %v", r)
			rt.finishMessage(state, streamID, 500, fmt.Sprintf("internal error: %v", r))
		}
	}()

	// Gate: if the startup goroutine hasn't finished the first DB→FS skill sync
	// yet, wait for it here before InitSkills reads the registry off disk.
	// sync.Once makes this a cheap no-op after the first successful pass.
	rt.ensureAISkillsSynced()

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
		rt.finishMessage(state, streamID, 400, "no AI agent configured for use_case=chat")
		return
	}

	var llmCfg *models.AILLMConfig
	if agent.LLMConfigId > 0 {
		llmCfg, err = models.AILLMConfigGetById(rt.Ctx, agent.LLMConfigId)
		if err != nil {
			logger.Warningf("[Assistant] load agent LLM config id=%d failed: %v", agent.LLMConfigId, err)
		}
	}
	// Fall back to the default LLM when the agent has no binding (LLMConfigId=0,
	// e.g. the auto-created default-chat-agent) or when its binding no longer
	// resolves (the referenced LLM was deleted).
	if llmCfg == nil {
		llmCfg, err = models.AILLMConfigPickDefault(rt.Ctx)
		if err != nil {
			logger.Warningf("[Assistant] pick default LLM config failed: %v", err)
		}
	}
	if llmCfg == nil {
		rt.finishMessage(state, streamID, 400, "no LLM configured, please configure one in system settings")
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
		rt.finishMessage(state, streamID, 500, fmt.Sprintf("failed to create LLM client: %v", err))
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
		DBCtx:         rt.Ctx,
		GetPromClient: func(dsId int64) prom.API { return rt.PromClients.GetCli(dsId) },
		GetSQLDatasource: func(dsType string, dsId int64) (datasource.Datasource, bool) {
			return dscache.DsCache.Get(dsType, dsId)
		},
		FilterDatasources: rt.DatasourceCache.DatasourceFilter,
	}

	if handler.Preflight != nil {
		user, uerr := models.UserGetById(rt.Ctx, userId)
		if uerr != nil || user == nil {
			rt.finishMessage(state, streamID, 500, "failed to resolve user for preflight")
			return
		}
		halt, preResps, perr := handler.Preflight(parentCtx, toolDeps, chatReq, user)
		if perr != nil {
			logger.Warningf("[Assistant] preflight error for action_key=%s: %v", actionKey, perr)
		}
		if halt {
			rt.finishHaltedMessage(state, streamID, history, preResps)
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
			rt.finishMessage(state, streamID, -2, "cancelled")
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
				_ = rt.streamBus.Append(parentCtx, msg.ChatID, streamID, aiagent.StreamMessage{V: delta, P: "reason"})
			}
		case aiagent.StreamTypeText:
			delta := chunk.Delta
			if delta == "" {
				delta = chunk.Content
			}
			if delta != "" {
				fullReasoning += delta
				_ = rt.streamBus.Append(parentCtx, msg.ChatID, streamID, aiagent.StreamMessage{V: delta, P: "reason"})
			}
		case aiagent.StreamTypeToolCall:
			executedTools = true
			step := "Using tools..."
			if chunk.Content != "" {
				step = chunk.Content
			}
			state.Update(parentCtx, func(m *models.AssistantMessage) {
				m.CurStep = step
				m.ExecutedTools = true
			})
		case aiagent.StreamTypeToolResult:
			state.Update(parentCtx, func(m *models.AssistantMessage) {
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
			rt.finishMessage(state, streamID, 500, errMsg)
			return
		case aiagent.StreamTypeDone:
			if chunk.Content != "" {
				fullContent = chunk.Content
				_ = rt.streamBus.Append(parentCtx, msg.ChatID, streamID, aiagent.StreamMessage{V: chunk.Content, P: "content"})
			}
		}
	}

	// 用 Background 而非 parentCtx：cancel / 超时路径下 parentCtx 已经 Done，
	// pipe.Exec(parentCtx) 会直接返回 context.Canceled，finish marker 写不进
	// stream，所有还连着的 SSE 消费者只能等 /stream handler 里的 orphan watchdog
	// 兜底。终态写入和 finishMessage 保持一致用 Background。
	_ = rt.streamBus.Finish(context.Background(), msg.ChatID, streamID)

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

	// Cancel race guard：/cancel handler 已经把 cancelled 的终态写进 Redis 快照 +
	// DB data + DB status=-2。如果 owner 在收到 cancel 信号之前正好把 chunks 跑完
	// 走到这里，下面的 AssistantMessageSet / state.Persist 会把成功态盖回去——
	// /detail 看到的是成功，但 history 因为 status=-2 把它过滤掉，前后矛盾。
	// 见 cancel marker 就直接 return，让 cancel handler 的写入是权威终态。
	if cancelled, _ := models.MsgCancelExists(context.Background(), rt.Redis, msg.ChatID, msg.SeqID); cancelled {
		return
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
	// Redis 上的 msg 快照刷一次最终态。完结后 24h TTL 自然过期，detail 接口
	// 在过期后会 fallback 到 DB 读取。用 Background 兜底——parentCtx 此时可能因
	// cancel/超时而 Done。
	state.Persist(context.Background())
}

func (rt *Router) finishMessage(state *MessageState, streamID string, errCode int, errMsg string) {
	msg := state.Msg()
	_ = rt.streamBus.Finish(context.Background(), msg.ChatID, streamID)

	// 同 success 路径：/cancel 已经把 cancelled 终态写到 Redis + DB 时，这里如果再
	// 用 errCode (常见 500/通用错误) 覆盖 DB data，会造成 /detail 看到错误码但
	// history 因 status=-2 过滤掉它的前后不一致。见 cancel 标志就让 /cancel 的
	// 写入做权威。
	if cancelled, _ := models.MsgCancelExists(context.Background(), rt.Redis, msg.ChatID, msg.SeqID); cancelled {
		return
	}

	msg.IsFinish = true
	msg.CurStep = ""
	msg.ErrCode = errCode
	msg.ErrMsg = errMsg

	if err := models.AssistantMessageSet(rt.Ctx, *msg); err != nil {
		logger.Errorf("[Assistant] failed to save error message: %v", err)
	}
	state.Persist(context.Background())
}

// finishHaltedMessage ends the turn without running the agent (used by preflight
// hooks that ask the user for missing context). Responses are attached as normal
// success-path responses, streamID is wired to the first one, and the chat
// history records only the user's input.
func (rt *Router) finishHaltedMessage(state *MessageState, streamID string, history []aiagent.ChatMessage, responses []models.AssistantMessageResponse) {
	msg := state.Msg()
	_ = rt.streamBus.Finish(context.Background(), msg.ChatID, streamID)

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
	state.Persist(context.Background())
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

	// 优先读 Redis 上的进行中/最近完结快照——任意实例都能拿到 owner 实时写入的
	// CurStep / 部分 Response / IsFinish。
	if snap, err := models.MsgStateGet(c, rt.Redis, req.ChatID, req.SeqID); err == nil && snap != nil {
		// Orphan 收敛：snap 看上去还在执行中，但 ChatLock 已经不在了——可能是
		// owner 真崩了，也可能是 owner 刚刚正常收尾（state.Persist 写完 Redis
		// IsFinish=true 之后才 defer lock.Release）。这里先做完两次 Redis 读再下结论：
		//
		//   1. ChatLockHeld 出错（Redis 抖动）保守不收敛——别把活的 owner 误判 orphan
		//   2. 锁不在 → 重新拉一次 snap。owner 正常收尾 / cancel handler 已经把终态
		//      写进 Redis 时，第二次读会拿到 IsFinish=true 的最终态，直接返回，不会
		//      再用第一次读到的旧 snap 去 AssistantMessageSet 反向覆盖 DB 成功态
		//   3. 第二次读还是 IsFinish=false → 真 orphan，但还要分两种：
		//      a) MsgCancelExists=true：cancel handler 在 race 中没赢，但它一定会
		//         继续把 cancelled 终态写进 DB+Redis。这里只把响应渲染成 cancelled，
		//         别 AssistantMessageSet / MsgStateDelete 去抢占 cancel handler 的写入
		//      b) 否则才是真崩：固化终态到 DB，给 stream 写 finish marker，删 Redis 快照
		if !snap.IsFinish {
			held, lerr := models.ChatLockHeld(c, rt.Redis, req.ChatID)
			if lerr != nil {
				logger.Warningf("[Assistant] ChatLockHeld chat=%s seq=%d: %v", req.ChatID, req.SeqID, lerr)
			} else if !held {
				// Lost-update 防御：用最新的 snap 替换我们手里那份可能已陈旧的副本
				if fresh, ferr := models.MsgStateGet(c, rt.Redis, req.ChatID, req.SeqID); ferr == nil && fresh != nil {
					snap = fresh
				}
				if !snap.IsFinish {
					cancelled, _ := models.MsgCancelExists(c, rt.Redis, req.ChatID, req.SeqID)
					snap.IsFinish = true
					snap.CurStep = ""
					if cancelled {
						if snap.ErrCode == 0 {
							snap.ErrCode = int(models.MessageStatusCancel)
							snap.ErrMsg = "cancelled by user"
						}
						// 不动 DB / 不删 Redis：cancel handler 的写入是权威终态
					} else {
						if snap.ErrCode == 0 {
							snap.ErrCode = 500
							snap.ErrMsg = "owner instance lost, message aborted"
						}
						if err := models.AssistantMessageSet(rt.Ctx, *snap); err != nil {
							logger.Warningf("[Assistant] orphan persist DB chat=%s seq=%d: %v", req.ChatID, req.SeqID, err)
						}
						for _, r := range snap.Response {
							if r.StreamID == "" {
								continue
							}
							if err := rt.streamBus.Finish(c, req.ChatID, r.StreamID); err != nil {
								logger.Warningf("[Assistant] orphan streamBus.Finish chat=%s stream=%s: %v", req.ChatID, r.StreamID, err)
							}
						}
						_ = models.MsgStateDelete(c, rt.Redis, req.ChatID, req.SeqID)
					}
				}
			}
		}
		ginx.NewRender(c).Data(snap, nil)
		return
	} else if err != nil {
		logger.Warningf("[Assistant] MsgStateGet chat=%s seq=%d: %v", req.ChatID, req.SeqID, err)
	}

	// Fallback 历史消息：Redis 24h TTL 已过期，从 DB 读取最终持久化结果。
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

	// 检查目标消息是否还在进行中：未完结或 key 已不在则不能取消
	snap, err := models.MsgStateGet(c, rt.Redis, req.ChatID, req.SeqID)
	ginx.Dangerous(err)
	if snap == nil || snap.IsFinish {
		ginx.Bomb(http.StatusNotFound, "message not executing or not found")
		return
	}

	// 双通道通知 owner：(1) SET 取消标志兜底，对抗 pubsub 偶发漏发；
	// (2) PUBLISH 即时通知，让 owner 在毫秒级触发本地 cancel。
	if err := models.MsgCancelMark(c, rt.Redis, req.ChatID, req.SeqID); err != nil {
		logger.Warningf("[Assistant] MsgCancelMark chat=%s seq=%d: %v", req.ChatID, req.SeqID, err)
	}
	if err := rt.pubsubBus.Publish(c, models.MsgCancelChannel(req.ChatID, req.SeqID), ""); err != nil {
		logger.Warningf("[Assistant] cancel publish chat=%s seq=%d: %v", req.ChatID, req.SeqID, err)
	}

	// 自闭环收尾：不依赖 owner 实例还活着。owner 健在时它的 finishMessage 也会写
	// 终态，与下面这次写入互为幂等覆盖；owner 崩溃 / 卡死时，下面这次写入就是权威
	// 终态——/detail 立即读到 cancelled，/stream 的消费者读到 finish marker 后退出。
	var streamID string
	for _, r := range snap.Response {
		if r.StreamID != "" {
			streamID = r.StreamID
			break
		}
	}
	if streamID != "" {
		if err := rt.streamBus.Finish(c, req.ChatID, streamID); err != nil {
			logger.Warningf("[Assistant] cancel streamBus.Finish chat=%s stream=%s: %v", req.ChatID, streamID, err)
		}
	}

	// 反向竞态防护（race A）：① 校验 → ③ 设 marker → ④ 写终态 之间约 ~5ms，owner
	// 完全可能在这个窗口里走完 success 路径写完 IsFinish=true 的权威终态。如果直接
	// 用 ① 读到的旧 in-flight snap 写 Redis/DB，会把 owner 刚写完的成功结果反向
	// 覆盖成 cancelled + 空 Response，再叠加 status=-2 让 history 过滤掉这条消息——
	// 用户已经从 SSE 看完全文，点 cancel 后却"凭空消失"。
	//
	// 这里 re-read 一次：发现 owner 已经收尾就 no-op，cancel 视为"来晚了"。注意 ③
	// MsgCancelMark 已经设上，owner 端 MessageState.Update/Persist 内部的 guard 会
	// 让此后任何 owner 写 Redis 都跳过——也就是 fresh==in-flight 时这里继续往下写
	// 不会被 owner 反向再覆盖 Redis。
	if fresh, ferr := models.MsgStateGet(c, rt.Redis, req.ChatID, req.SeqID); ferr != nil {
		logger.Warningf("[Assistant] cancel re-read chat=%s seq=%d: %v", req.ChatID, req.SeqID, ferr)
	} else if fresh != nil && fresh.IsFinish {
		ginx.NewRender(c).Message(nil)
		return
	}

	snap.IsFinish = true
	snap.CurStep = ""
	snap.ErrCode = int(models.MessageStatusCancel)
	snap.ErrMsg = "cancelled by user"
	if err := models.MsgStateSet(c, rt.Redis, snap); err != nil {
		logger.Warningf("[Assistant] cancel MsgStateSet chat=%s seq=%d: %v", req.ChatID, req.SeqID, err)
	}

	// 把 cancelled 的 data/extra 也写回 DB——光更新 status 列的话，Redis TTL 过期后
	// /detail fallback 到 DB 会拿到最初创建时的 is_finish=false/err_code=0 旧行。
	// 同时 owner 那边在最终 AssistantMessageSet 之前会检查 MsgCancelExists，看到取消
	// 标志就直接 return，不会再覆盖这里写的 cancelled data。
	if err := models.AssistantMessageSet(rt.Ctx, *snap); err != nil {
		logger.Warningf("[Assistant] cancel AssistantMessageSet chat=%s seq=%d: %v", req.ChatID, req.SeqID, err)
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

	// streamID 内嵌 chatID，格式 "<chatID>:<uuid>"——assistantMessageNew 写入。
	// 用 chatID 作为 hash tag 才能在 Cluster 模式下定位到对应 stream key。
	chatID := parseChatIDFromStreamID(req.StreamID)
	if chatID == "" {
		ginx.Bomb(http.StatusBadRequest, "invalid stream_id format")
		return
	}
	// seqID 可能是 0——旧格式 streamID 不带 seqID，watchdog 此时回退到 chat 粒度的
	// ChatLockHeld 判定（不能区分"我们这条 message 已经孤儿但同 chat 又起了新 message"
	// 的边角，但这是旧 streamID 在 in-flight 期间不可避免的限制）。
	seqID := parseSeqIDFromStreamID(req.StreamID)

	// SSE responses can outlive http.Server.WriteTimeout (40s by default), which
	// would otherwise close the underlying TCP connection mid-stream. Clear the
	// per-connection write deadline for this handler only.
	if rc := http.NewResponseController(c.Writer); rc != nil {
		_ = rc.SetWriteDeadline(time.Time{})
	}

	// Tie the reader's lifetime to the HTTP request so a client disconnect
	// (or normal handler return) releases the StreamBus consumer goroutine
	// instead of leaking it until Finish/timeout.
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Stream key 不存在时立即收尾——避免对非法 / 已 TTL 过期的 streamID 做
	// XREAD BLOCK 而无限挂起 goroutine + Redis 连接。assistantMessageNew 会同步
	// 种入 init marker，所以"合法但 owner 还没首包"的情况此处也能通过。
	exists, err := rt.streamBus.Exists(ctx, chatID, req.StreamID)
	if err != nil {
		logger.Warningf("[Assistant] streamBus.Exists chat=%s stream=%s: %v", chatID, req.StreamID, err)
	}
	if err == nil && !exists {
		c.Stream(func(w io.Writer) bool {
			fmt.Fprintf(w, "event: finish\ndata:\n\n")
			c.Writer.Flush()
			return false
		})
		return
	}

	ch := rt.streamBus.Read(ctx, chatID, req.StreamID)

	// Orphan watchdog：owner 在 Init 之后、Finish 之前崩溃时 stream 里永远不会
	// 出现 finishMarker，单靠 XREAD BLOCK + 24h TTL 会让 SSE 消费者长时间挂着。
	//
	// 优先用 message 粒度的信号 MsgStateGet(chatID, seqID).IsFinish——这能精准对应
	// "我们这条 message 是不是结束了"，覆盖以下两种 case：
	//   1) owner 正常 finalize 写过 IsFinish=true 但 SSE 这边漏了 finishMarker；
	//   2) owner 真崩了 + 同 chat 又起了新 message，新 message 的 ChatLock 让旧的
	//      chat 粒度 ChatLockHeld 判定永远 true，老 watchdog 在这场景下失效。
	//
	// 若 snap 直接 nil（24h TTL 过期）也视作"早就该结束了"。snap 还在 in-flight
	// (IsFinish=false) 时，再退一步用 ChatLockHeld 兜——锁不在意味着没有任何 owner
	// 在为本 chat 工作，旧 streamID 也该收尾。
	//
	// seqID==0 (legacy streamID) 时跳过 message 级判定，行为退化到老逻辑。
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if seqID > 0 {
					snap, serr := models.MsgStateGet(ctx, rt.Redis, chatID, seqID)
					if serr != nil {
						// Redis 抖动，下个 tick 再试，不要误杀
						continue
					}
					if snap == nil || snap.IsFinish {
						if err := rt.streamBus.Finish(ctx, chatID, req.StreamID); err != nil {
							logger.Warningf("[Assistant] orphan watchdog Finish chat=%s stream=%s: %v", chatID, req.StreamID, err)
						}
						return
					}
					// snap 还在 in-flight，落到下面的 lock 判定
				}
				held, herr := models.ChatLockHeld(ctx, rt.Redis, chatID)
				if herr != nil {
					continue
				}
				if !held {
					if err := rt.streamBus.Finish(ctx, chatID, req.StreamID); err != nil {
						logger.Warningf("[Assistant] orphan watchdog Finish chat=%s stream=%s: %v", chatID, req.StreamID, err)
					}
					return
				}
			}
		}
	}()

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
