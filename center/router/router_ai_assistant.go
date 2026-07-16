package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/chat"
	"github.com/ccfos/nightingale/v6/aiagent/llm"
	_ "github.com/ccfos/nightingale/v6/aiagent/tools" // register builtin tools
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
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

	chat, err := models.AssistantChatCheckOwner(rt.Ctx, req.ChatID, me.Id)
	ginx.Dangerous(err)

	// Capture X-Language before the gin.Context goes out of scope — the runner
	// goroutine outlives this handler. Used to pin the agent's natural-language
	// output to the UI language (see chat.LanguageDirective).
	lang := c.GetHeader("X-Language")

	result, status, err := rt.StartAssistantMessage(me.Id, chat, req.Query, lang)
	if err != nil {
		// Business errors (status != 0, e.g. 409 busy) keep their explicit
		// status code via Bomb. System errors (status == 0) fall through to
		// Dangerous so they emerge as the n9e-standard "200 + {err: ...}"
		// envelope — same shape as every other handler in this codebase, so
		// the fe error interceptor and 5xx alerts behave consistently.
		if status != 0 {
			ginx.Bomb(status, "%s", err.Error())
			return
		}
		ginx.Dangerous(err)
		return
	}

	ginx.NewRender(c).Data(gin.H{
		"chat_id": result.ChatID,
		"seq_id":  result.SeqID,
	}, nil)
}

func (rt *Router) processAssistantMessage(parentCtx context.Context, parentCancel context.CancelFunc, lock *models.ChatLock, state *MessageState, streamID string, userId int64, lang string) {
	msg := state.Msg()

	// Timing instrumentation: capture per-phase durations so we can answer
	// "where did the 8s go" without re-running the request. All durations are
	// relative to tStart (entry of this goroutine). Phases that don't run
	// (e.g. preflight when handler.Preflight == nil) stay at 0.
	tStart := time.Now()
	var (
		tHistoryLoaded time.Duration
		tLLMReady      time.Duration
		tIntent        time.Duration
		intentMethod   string // "fast" | "import" | "form" | "default"（全确定性，零 LLM）
		tValidatePre   time.Duration
		tAgentStart    time.Duration
		tFirstToken    time.Duration
		tStreamDone    time.Duration
		tPersisted     time.Duration
	)

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

	// ① Load multi-turn history + route state early（路由延续与表单继承都依赖它们）
	var history []aiagent.ChatMessage
	var prevRoute *models.ConversationRoute
	var prevPending *models.PendingInterrupt
	if msg.SeqID > 1 {
		prevMsg, _ := models.AssistantMessageGet(rt.Ctx, msg.ChatID, msg.SeqID-1)
		if prevMsg != nil {
			// Route state: the previous turn's resolved action。仅表单提交轮
			// （上轮 AwaitingForm + 本轮带 action.param）确定性继承 ActionKey；
			// 普通延续轮重新解析（落回 general_chat）。
			prevRoute = prevMsg.Extra.Route
			// Pending interrupt: the previous turn ended awaiting user confirmation.
			prevPending = prevMsg.Extra.Pending
			if len(prevMsg.Extra.HistoryMessages) > 0 {
				// History is a structured transcript envelope.
				// No backward-compat with the old fullContent-only format: a blob that fails to
				// decode just yields empty history.
				var env aiagent.TranscriptEnvelope
				if err := json.Unmarshal(prevMsg.Extra.HistoryMessages, &env); err != nil {
					logger.Warningf("[Assistant] failed to unmarshal history for chat=%s seq=%d: %v", msg.ChatID, msg.SeqID-1, err)
				} else {
					history = env.Messages
				}
			}
		}
	}
	tHistoryLoaded = time.Since(tStart)

	// 人在环 resume 短路：上一轮以待确认中断收尾时，approve/reject 由运行时
	// 处理（approve = 直接重放工具 apply 腿，分层裁决见 router_ai_interrupt.go），
	// 不进入 action 解析与 agent 流程；语义不明的回复回归正常流程，pending 不再携带
	// （旧提案由工具自身 TTL/单次消费门作废）。
	if prevPending != nil && rt.tryResumePending(state, streamID, prevPending, history, prevRoute, lang) {
		return
	}

	// ② Create LLM client（agent 执行用）
	// Always use "chat" useCase to find the agent config for the LLM client.
	agent, err := models.AIAgentGetByUseCase(rt.Ctx, "chat")
	if err != nil || agent == nil {
		rt.finishMessage(state, streamID, 400, "no AI agent configured for use_case=chat")
		return
	}

	llmCfg := rt.resolveChatLLMConfig(agent)
	if llmCfg == nil {
		// 用 409 作为业务错误码（写进 message.ErrCode），前端读 body.dat.err_code
		// 识别"未配置 LLM"这条特定错误，弹"去配置"引导而非通用 toast。HTTP 状态
		// 码保持 200——/detail 接口对所有完成态消息都是 200，错误细节走 body。
		rt.finishMessage(state, streamID, http.StatusConflict, "no LLM configured, please configure one in system settings")
		return
	}

	llmClient, llmCallTimeout, err := rt.chatLLMClient(llmCfg)
	if err != nil {
		rt.finishMessage(state, streamID, 500, fmt.Sprintf("failed to create LLM client: %v", err))
		return
	}
	// agentTotalTimeout caps the entire tool loop across all tool calls. Multi-turn
	// agent flows (e.g. dashboard creation: list_busi_groups → list_datasources →
	// list_files → read_file → create_dashboard) easily run 7+ iterations, so the
	// agent budget must be several times the per-call (llmCallTimeout) budget.
	agentTotalTimeout := llmCallTimeout * 5
	if agentTotalTimeout < 5*60*1000 {
		agentTotalTimeout = 5 * 60 * 1000
	}
	tLLMReady = time.Since(tStart)

	// ③ Resolve action key —— 全部确定性信号，零 LLM（LLM 意图分类已删除：
	// 旁路分类器看不到完整语境、串行 1.4-3s、分错即漏门；
	// 开放输入交给通用 agent + 技能目录自取，确定性门由工具层兜底。
	// 前端 action.key 通道已废弃——fe 发送前一律剥掉 key，
	// 专用 action 条目随之删除；未来非对话场景需要时再加回）：
	//   1. Creation fast-path — 创建动词命中 → creation（保住零 LLM 即时弹表单的 UX）
	//   2. 表单延续 — 上轮以 form_select 收尾且本轮带 action.param（= 表单提交），
	//      确定性继承上轮 action，替代原先靠分类器判"延续"
	//   3. 默认 — general_chat 通用路径
	tIntentStart := time.Now()
	actionKey, method := resolveActionKey(msg.Query.Content, len(msg.Query.Action.Param), prevRoute)
	intentMethod = method
	tIntent = time.Since(tStart)
	intentDur := time.Since(tIntentStart)

	handler, ok := chat.Lookup(actionKey)
	if !ok {
		// 陈旧 key——唯一来源是旧版本持久化的 Route.ActionKey 经 form 分支
		// 继承而来（对应 action 已随路由收缩删除），兜底 general_chat
		actionKey = string(models.ActionKeyGeneralChat)
		handler, _ = chat.Lookup(actionKey)
	}

	logger.Infof("[Assistant] chat=%s seq=%d action_key=%s content=%q",
		msg.ChatID, msg.SeqID, actionKey, msg.Query.Content)
	logger.Infof("[Assistant.Timing] chat=%s seq=%d phase=intent_resolved method=%s dur=%dms total=%dms action_key=%s",
		msg.ChatID, msg.SeqID, intentMethod, intentDur.Milliseconds(), tIntent.Milliseconds(), actionKey)

	// Build AIChatRequest for reusing existing action logic
	chatReq := &chat.AIChatRequest{
		ActionKey: actionKey,
		UserInput: msg.Query.Content,
		Context:   make(map[string]interface{}),
		Language:  lang,
		ChatID:    msg.ChatID,
		SeqID:     msg.SeqID,
	}

	// Merge action.param into context — handlers consume Context as a generic
	// map[string]interface{} (see ctxInt64 in aiagent/chat/actions.go), so
	// param flows through verbatim. Adding a new param requires no router
	// changes; the consuming handler just reads its key from req.Context.
	for k, v := range msg.Query.Action.Param {
		chatReq.Context[k] = v
	}

	// 确定性上下文富化（原 edit 专属的 PreflightEdit，泛化到所有 action）：
	// 把用户粘贴的 /alert-rules/edit/<id>、/dashboards/<id> URL 提取成
	// rule_id/dashboard_id 注入 Context，通用路径上的编辑请求免一轮工具解析。
	chat.EnrichContextFromText(chatReq)

	// ④ Preflight — hard gate. May halt the turn and emit structured responses
	// (e.g. ask the user to pick a busi group before a creation skill runs).
	toolDeps := rt.buildToolDeps()

	// Resolve the requesting user once: preflight needs it, and MCP-server team
	// scoping (buildMCPConfigForAgent) filters bound servers per this user.
	me, uerr := models.UserGetById(rt.Ctx, userId)
	if uerr != nil || me == nil {
		rt.finishMessage(state, streamID, 500, "failed to resolve user")
		return
	}

	// 私有 skill 可见性：一次算出当前用户无权访问的私有 skill 名单，既用于运行时
	// 加载层拦截（toolDeps：load_skill / run_skill_script 按名读取前校验），也用于
	// 技能目录展示过滤（skillCfg.HiddenSkillNames，见下）。fail closed 见
	// hiddenSkillNamesForUser。
	hiddenSkills, denySkills := rt.hiddenSkillNamesForUser(userId)
	toolDeps.DenyAllSkills = denySkills
	toolDeps.HiddenSkillNames = make(map[string]struct{}, len(hiddenSkills))
	for _, n := range hiddenSkills {
		toolDeps.HiddenSkillNames[n] = struct{}{}
	}

	if handler.Preflight != nil {
		halt, preResps, perr := handler.Preflight(parentCtx, toolDeps, chatReq, me)
		if perr != nil {
			logger.Warningf("[Assistant] preflight error for action_key=%s: %v", actionKey, perr)
		}
		if halt {
			tValidatePre = time.Since(tStart)
			logger.Infof("[Assistant.Timing] chat=%s seq=%d phase=preflight_halted total=%dms",
				msg.ChatID, msg.SeqID, tValidatePre.Milliseconds())
			// Persist the resolved action as route state so the follow-up turn
			// (e.g. the user filling the form this halt asked for) inherits it.
			// AwaitingForm（表单收尾）让提交轮被确定性路由回本 action。
			rt.finishHaltedMessage(state, streamID, history, preResps,
				&models.ConversationRoute{ActionKey: actionKey, AwaitingForm: hasFormSelect(preResps)}, "")
			return
		}
		tValidatePre = time.Since(tStart)
		logger.Infof("[Assistant.Timing] chat=%s seq=%d phase=preflight_pass total=%dms action_key=%s",
			msg.ChatID, msg.SeqID, tValidatePre.Milliseconds(), actionKey)
	} else {
		tValidatePre = time.Since(tStart)
	}

	// Select tools
	var tools []aiagent.AgentTool
	if handler.SelectTools != nil {
		toolNames := handler.SelectTools(chatReq)
		if toolNames != nil {
			tools = aiagent.GetBuiltinToolDefs(toolNames)
		}
	}

	// general_chat 子模式分类器（knowledge/data_query 的二次 LLM 分类）已随路由
	// 收缩一并删除：通用路径成为主路径后它会在每条开放消息上多烧一次 LLM 调用；
	// "该不该查文档/查数据"由主模型按 buildGeneralChatPrompt 的工具纪律自行决定
	// （search_n9e_docs 恒在工具集内）。

	userPrompt := ""
	if handler.BuildPrompt != nil {
		userPrompt = handler.BuildPrompt(chatReq)
	}
	// Pin LLM output to the UI language. Appended AFTER the action-specific
	// prompt so it lands at the tail of the agent's system instruction — the
	// position LLMs weight highest for "respond in X" directives. Empty lang
	// returns "" so we fall back to the LLM's auto-detection behavior.
	userPrompt += chat.LanguageDirective(lang)
	// System-level safety net: surface the front-end context map to the LLM
	// even when BuildPrompt doesn't read every key (see chat/context_dump.go
	// for rationale). Empty context renders to "" so unaffected actions pay
	// nothing.
	userPrompt += chat.ContextDump(chatReq.Context)

	inputs := buildAgentInputs(chatReq, userId, msg.ChatID, msg.SeqID)

	// 用 UserPromptRendered 而非 UserPromptTemplate：handler.BuildPrompt 已经用
	// fmt.Sprintf 把 msg.Query.Content 原样拼进 userPrompt，不能再经 text/template
	// 解析——否则用户问 "告警模板怎么写 {{ .Alertname }}" 会让 Parse 失败，整轮 500。
	//
	// Skills / MCP 绑定：agent.SkillIds/MCPServerIds 非空时走"精确注入"路径
	// （SkillNames + 固定 MCP server 列表），空则不预载——系统提示词常驻技能目录，
	// 模型经 load_skill 自取。action handler 若声明了 RequiredSkills，则覆盖上述两者——见 resolveSkillConfig。
	skillCfg := rt.resolveSkillConfig(handler, chatReq, agent)
	// 私有 skill 仅对授权团队可见：把当前用户在 AI 对话里看不到的私有 skill
	// 从常驻技能目录里过滤掉（与运行时加载层同一份名单，见上 hiddenSkills）。
	// denySkills 为 fail-closed 兜底：无法算出名单时目录留空 + 拒绝所有预载/注入。
	skillCfg.HiddenSkillNames = hiddenSkills
	skillCfg.DenyAllSkills = denySkills

	// mcpOAuthWatch: 收集需要（重新）授权的 MCP —— 既有装配前本地预检就判定不可用的，
	// 也有凭据被服务端撤销、要等 agent 真正连接才暴露的。本轮用不上它们的工具，末尾以
	// 授权按钮提示用户（否则工具只是静默消失，用户无从知道该去授权）。必须在 agent
	// 跑完之后再读，运行时发现的那批才在里面。
	mcpCfg, mcpOAuthWatch := rt.buildMCPConfigForAgent(agent, me)

	agentRunner := aiagent.NewAgent(&aiagent.AgentConfig{
		Tools:              tools,
		Timeout:            agentTotalTimeout,
		Stream:             true,
		UserPromptRendered: userPrompt,
		GuidedFollowup:     true, // 交互式 chat：末尾给可点选的"下一步"建议
		Skills:             skillCfg,
		MCP:                mcpCfg,
		HistoryBudgetBytes: historyBudgetFromContextLength(llmCfg.ExtraConfig.ContextLength),
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
	tAgentStart = time.Since(tStart)

	// Consume stream chunks
	var fullContent string
	var segAcc segmentAccumulator // 按到达顺序累积 reasoning/content 展示段，终态按段分块持久化
	finalBodyStreamed := false    // Done(content_streamed) 置位：最终轮正文已逐 token 流出，其原始段由权威 markdown 替代
	var createdAlertRules []string
	var createdDashboards []string
	var turnMsgs []aiagent.ChatMessage    // 本轮工具调用轮 + 结果轮，用于持久化结构化 transcript
	var pendingI *models.PendingInterrupt // 非空 = 本轮以人在环中断收尾（Step 4）
	var interruptForm string              // input 类中断附带的 form_select 载荷（与 preflight 同契约）
	executedTools := false
	firstTokenSeen := false
	markFirstToken := func(kind string) {
		if firstTokenSeen {
			return
		}
		firstTokenSeen = true
		tFirstToken = time.Since(tStart)
		logger.Infof("[Assistant.Timing] chat=%s seq=%d phase=first_token kind=%s ttft=%dms total=%dms",
			msg.ChatID, msg.SeqID, kind, (tFirstToken - tAgentStart).Milliseconds(), tFirstToken.Milliseconds())
	}

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
				markFirstToken("thinking")
				segAcc.append(segmentKindReasoning, delta)
				_ = rt.streamBus.Append(parentCtx, msg.ChatID, streamID, aiagent.StreamMessage{V: delta, P: "reason"})
			}
		case aiagent.StreamTypeContent:
			// 正文 token 直接归 content 通道。同时累加到 fullContent——Done.Content
			// 虽是权威正文，但异常断流时 Done 可能缺席，router 末尾写最终
			// AssistantMessageResponse 仍需有正文可用。
			delta := chunk.Delta
			if delta == "" {
				delta = chunk.Content
			}
			if delta != "" {
				markFirstToken("content")
				fullContent += delta
				segAcc.append(segmentKindContent, delta)
				_ = rt.streamBus.Append(parentCtx, msg.ChatID, streamID, aiagent.StreamMessage{V: delta, P: "content"})
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
			// 轮边界：收口当前展示段（与下方 P:"step" 帧同点位）。下一轮的思考/正文
			// 开新段，否则跨轮思考在持久化块里粘成一段。
			segAcc.closeCurrent()
			state.Update(parentCtx, func(m *models.AssistantMessage) {
				m.CurStep = "Processing tool result..."
			})
			// Iteration boundary: push a P:"step" frame so downstream
			// consumers (A2A bridge) can close out the current reasoning
			// artifact and start a new one — without this, multi-step
			// thoughts render as one undelimited blob.
			stepText := "tool_result"
			if chunk.Metadata != nil {
				if toolName, _ := chunk.Metadata["tool"].(string); toolName != "" {
					stepText = "tool_result:" + toolName
				}
			}
			_ = rt.streamBus.Append(parentCtx, msg.ChatID, streamID, aiagent.StreamMessage{V: stepText, P: "step"})
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
					case "import_alert_rule_template":
						// Batch import returns a summary with a "cards" array of
						// per-rule payloads; fan them out so each imported rule
						// gets its own alert_rule UI card.
						createdAlertRules = append(createdAlertRules, extractImportedAlertRuleCards(obs)...)
					case "create_dashboard", "import_dashboard_template":
						createdDashboards = append(createdDashboards, obs)
					}
				}
			}
		case aiagent.StreamTypeTranscript:
			// 本轮工具循环产生的规范消息（assistant 工具调用轮 + tool 结果轮）。收集
			// 起来在末尾持久化为下一轮可回放的结构化 transcript，使工具产物（如
			// proposal_id）跨轮对模型可见。纯 agent→router 内部通道：不写 stream
			// bus、不转发 A2A。
			turnMsgs = append(turnMsgs, chunk.Transcript...)
		case aiagent.StreamTypeInterrupt:
			// 工具人在环中断（Step 4）：记录 Pending，本轮以确认文案收尾（文案经
			// Done chunk 落 content 通道）。approval 类下一轮由 tryResumePending
			// 确定性重放；input 类附带 form_select 表单（与 preflight 同一前端
			// 契约），下一轮带着表单值回归 agent 流程。
			kind, _ := chunk.Metadata["kind"].(string)
			toolName, _ := chunk.Metadata["tool"].(string)
			resumeArgs, _ := chunk.Metadata["resume_args"].(string)
			interruptForm, _ = chunk.Metadata["form"].(string)
			pendingI = &models.PendingInterrupt{
				Kind:       kind,
				Tool:       toolName,
				ResumeArgs: resumeArgs,
				Params:     inputs,
				Prompt:     chunk.Content,
				SeqID:      msg.SeqID,
			}
		case aiagent.StreamTypeError:
			errMsg := chunk.Error
			if errMsg == "" {
				errMsg = chunk.Content
			}
			rt.finishMessage(state, streamID, 500, errMsg)
			return
		case aiagent.StreamTypeDone:
			// Streamed path (Metadata.content_streamed): the body already
			// streamed through StreamTypeContent frame by frame, possibly
			// preceded by intermediate-round commentary. Done.Content is the
			// authoritative final body (final-round text only) — use it for
			// parsing/persistence, but do NOT re-append to the bus, or the
			// answer would render twice.
			if streamed, _ := chunk.Metadata["content_streamed"].(bool); streamed {
				finalBodyStreamed = true
				if chunk.Content != "" {
					fullContent = chunk.Content
				}
				break
			}
			// Non-streamed bodies (interrupt prompts, max-iteration partials):
			// never pushed to any channel, so Done.Content is the only copy —
			// adopt it and push to the content channel.
			if chunk.Content != "" {
				fullContent = chunk.Content
				_ = rt.streamBus.Append(parentCtx, msg.ChatID, streamID, aiagent.StreamMessage{V: chunk.Content, P: "content"})
			}
		}
	}

	tStreamDone = time.Since(tStart)

	// Push cards into the stream before the finish marker so stream-only
	// consumers (A2A) get them; the frontend ignores these and reads /detail.
	for _, ruleJSON := range createdAlertRules {
		if err := rt.streamBus.PublishResponse(context.Background(), msg.ChatID, streamID,
			models.AssistantMessageResponse{ContentType: models.ContentTypeAlertRule, Content: ruleJSON}); err != nil {
			logger.Warningf("[Assistant] PublishResponse alert_rule card chat=%s stream=%s: %v", msg.ChatID, streamID, err)
		}
	}
	for _, dashJSON := range createdDashboards {
		if err := rt.streamBus.PublishResponse(context.Background(), msg.ChatID, streamID,
			models.AssistantMessageResponse{ContentType: models.ContentTypeDashboard, Content: dashJSON}); err != nil {
			logger.Warningf("[Assistant] PublishResponse dashboard card chat=%s stream=%s: %v", msg.ChatID, streamID, err)
		}
	}

	// 人在环中断收尾：在 finish 前压一个 input_required 帧（V=确认问题文本）。
	// A2A bridge 据此把任务终态标成 TaskStateInputRequired，上游 agent 客户端
	// （fc-model-server 等）会把确认问题转给真人回答——否则确认请求被当成普通
	// 工具结果，上游模型自行代答"用户已确认"，审批门形同虚设。SSE 前端不认识
	// 此帧，照常忽略（确认 UI 由 /detail 渲染）。
	if pendingI != nil {
		prompt := pendingI.Prompt
		if prompt == "" {
			prompt = resumeText(lang, "请确认是否执行本次改动。", "Please confirm whether to apply this change.")
		}
		// approval 类附协议 token 提示（只上 stream 帧，仅 A2A 可见）：上游 agent
		// 拿到明确的回复格式，确认轮走零 NLP 的精确匹配；FE 用户看 form_select
		// 按钮，不需要这一行。
		if pendingI.Kind == aiagent.InterruptKindApproval {
			prompt += resumeText(lang,
				"\n\n（同意请回复 approve，取消请回复 reject；也可以直接说明新的修改要求。）",
				"\n\n(Reply exactly `approve` to apply or `reject` to cancel; or describe further changes.)")
		}
		if err := rt.streamBus.Append(context.Background(), msg.ChatID, streamID,
			aiagent.StreamMessage{P: aiagent.PhaseInputRequired, V: prompt}); err != nil {
			logger.Warningf("[Assistant] publish input_required chat=%s stream=%s: %v", msg.ChatID, streamID, err)
		}
	}

	// 用 Background 而非 parentCtx：cancel / 超时路径下 parentCtx 已经 Done，
	// pipe.Exec(parentCtx) 会直接返回 context.Canceled，finish marker 写不进
	// stream，所有还连着的 SSE 消费者只能等 /stream handler 里的 orphan watchdog
	// 兜底。终态写入和 finishMessage 保持一致用 Background。
	_ = rt.streamBus.Finish(context.Background(), msg.ChatID, streamID)

	// Build the authoritative final-answer markdown (the terminal block of the
	// response list).
	// Defensive: some models wrap a markdown final answer in a JSON envelope
	// like {"query": "## 结论\n..."}, conditioned by tool-call argument
	// examples. Unwrap before rendering as markdown so the user sees
	// real newlines instead of literal "\n" escapes.
	markdown := chat.UnwrapJSONEnvelope(fullContent)

	// general_chat 后置校验: 必须在 UnwrapJSONEnvelope 之后跑——若
	// fullContent 是 JSON envelope, append stamp 会破坏末尾 `}` 让 unwrap
	// 短路返回 raw JSON。
	if actionKey == string(models.ActionKeyGeneralChat) && markdown != "" {
		if clean, hits := chat.ValidateRestrictedGCOutput(markdown); !clean {
			logger.Warningf("[Assistant] general_chat post-check hit forbidden patterns: %v, appending stamp (chat=%s seq=%d)",
				hits, msg.ChatID, msg.SeqID)
			markdown += chat.BuildHallucinationStamp(lang, hits)
		}
	}

	// 按到达顺序把流式段落地成块：每轮思考/过渡语独立成块，末尾是权威 markdown
	// 终答块。块结构与流式视图同构（前端/A2A 按 P:"step" 帧切段），历史回放不再
	// 把多轮思考塌成单块、也不再丢失中间轮过渡语。
	responses := assembleSegmentResponses(segAcc.segments, markdown, streamID, finalBodyStreamed)

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

	// 需要（重新）授权的 MCP：给出授权按钮。此处才读 watch —— agent 已跑完，运行时
	// 才暴露的凭据失效（被撤销的 token、refresh 返回 invalid_grant）也已并入。这些
	// server 的工具本轮不可用，而失败是静默的，不提示的话用户只会觉得「工具凭空没了」。
	if mcpNeedsOAuth := mcpOAuthWatch.servers(); len(mcpNeedsOAuth) > 0 {
		responses = append(responses, models.AssistantMessageResponse{
			ContentType: models.ContentTypeMcpOAuth,
			Content:     buildMCPOAuthPayload(mcpNeedsOAuth),
			IsFinish:    true,
			IsFromAI:    true,
		})
	}

	// input 类工具中断：把工具产出的 form_select
	// 载荷渲染为结构化表单 response——与 preflight 表单同一前端契约。markdown
	// 正文（中断 Prompt）已在上面作为兜底文案存在，纯文本客户端（A2A）读它。
	if pendingI != nil && pendingI.Kind == aiagent.InterruptKindInput && interruptForm != "" {
		responses = append(responses, models.AssistantMessageResponse{
			ContentType: models.ContentTypeFormSelect,
			Content:     interruptForm,
			IsFinish:    true,
			IsFromAI:    true,
		})
	}

	// approval 类工具中断：附 approve/reject 二选一表单（结构化确认通道）。FE
	// 渲染成按钮，提交值经 action.param["approval"] 回传，下一轮零 NLP 直接
	// 裁决；自由文本回复仍走文本分类/agent 流程。
	if pendingI != nil && pendingI.Kind == aiagent.InterruptKindApproval {
		responses = append(responses, models.AssistantMessageResponse{
			ContentType: models.ContentTypeFormSelect,
			Content:     aiagent.BuildApprovalForm(lang, pendingI.Tool),
			IsFinish:    true,
			IsFromAI:    true,
		})
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

	// Persist the full structured transcript (not just the final answer): prior
	// history + the user query + this turn's tool-call/observation messages + a
	// single terminal assistant message holding the cleaned final answer. Tool
	// products (e.g. proposal_id) thus survive into the next turn's replay.
	newHistory := assembleTurnHistory(history, msg.Query.Content, turnMsgs, fullContent)
	env := aiagent.TranscriptEnvelope{SchemaVersion: aiagent.TranscriptSchemaVersion, Messages: newHistory}
	msg.Extra.HistoryMessages, _ = json.Marshal(env)

	// Persist route state. AwaitingForm 标记"本轮以表单收尾"——下一轮带
	// action.param 的提交据此确定性继承本 action（form 延续信号）；
	// 除此之外 ActionKey 不跨轮继承，普通延续轮重新解析。
	msg.Extra.Route = &models.ConversationRoute{
		ActionKey:    actionKey,
		AwaitingForm: pendingI != nil && pendingI.Kind == aiagent.InterruptKindInput && interruptForm != "",
	}
	// Persist the pending interrupt if this turn ended awaiting confirmation (Step 4, I5).
	msg.Extra.Pending = pendingI

	// Save to DB (UPSERT)
	if err := models.AssistantMessageSet(rt.Ctx, *msg); err != nil {
		logger.Errorf("[Assistant] failed to save message: %v", err)
	}
	// Redis 上的 msg 快照刷一次最终态。完结后 24h TTL 自然过期，detail 接口
	// 在过期后会 fallback 到 DB 读取。用 Background 兜底——parentCtx 此时可能因
	// cancel/超时而 Done。
	state.Persist(context.Background())
	tPersisted = time.Since(tStart)

	// One-line summary for easy log scanning. All durations are ms relative to
	// tStart (goroutine entry). "stream" = tStreamDone - tFirstToken (tail of
	// LLM output after first token); "persist" = tPersisted - tStreamDone.
	streamDur := int64(0)
	if tFirstToken > 0 {
		streamDur = (tStreamDone - tFirstToken).Milliseconds()
	}
	logger.Infof("[Assistant.Summary] chat=%s seq=%d action=%s total=%dms | history=%dms llm_client=%dms intent=%dms(%s) validate_pre=%dms agent_start=%dms ttft=%dms stream=%dms persist=%dms",
		msg.ChatID, msg.SeqID, actionKey,
		tPersisted.Milliseconds(),
		tHistoryLoaded.Milliseconds(),
		(tLLMReady - tHistoryLoaded).Milliseconds(),
		(tIntent - tLLMReady).Milliseconds(), intentMethod,
		(tValidatePre - tIntent).Milliseconds(),
		(tAgentStart - tValidatePre).Milliseconds(),
		(tFirstToken - tAgentStart).Milliseconds(),
		streamDur,
		(tPersisted - tStreamDone).Milliseconds(),
	)
}

// resolveChatLLMConfig 选取 chat 路径的有效 LLM 配置：is_default 优先于 agent
// 绑定。用户在 LLM 配置列表把某条标为"默认"，直觉预期就是对话立刻切到该模型——
// 若 agent 绑定优先，默认标记会被静默忽略，用户很难发现 chat 实际在用 Agent
// 设置页绑定的另一条配置。agent 绑定降级为兜底：没有任何配置标默认时才生效。
// 返回 nil = 无可用配置。chat 主流程与 approval 意图分类共用同一选取逻辑。
func (rt *Router) resolveChatLLMConfig(agent *models.AIAgent) *models.AILLMConfig {
	llmCfg, err := models.AILLMConfigPickDefault(rt.Ctx) // 内部已过滤 enabled
	if err != nil {
		logger.Warningf("[Assistant] pick default LLM config failed: %v", err)
	}
	if llmCfg != nil {
		return llmCfg
	}
	if agent == nil || agent.LLMConfigId <= 0 {
		return nil
	}
	llmCfg, err = models.AILLMConfigGetById(rt.Ctx, agent.LLMConfigId)
	if err != nil {
		logger.Warningf("[Assistant] load agent LLM config id=%d failed: %v", agent.LLMConfigId, err)
		return nil
	}
	// AILLMConfigGetById 是通用 getter，故意不过滤 enabled——管理后台编辑页要能
	// 查出已禁用的记录。但用在 chat 业务路径，命中已禁用配置时应当视为"agent 没
	// 可用 LLM"。
	if llmCfg != nil && !llmCfg.Enabled {
		logger.Infof("[Assistant] agent's bound LLM config id=%d is disabled", llmCfg.Id)
		return nil
	}
	return llmCfg
}

// chatLLMClient 由 LLM 配置构造（缓存的）客户端，并返回单次 LLM HTTP 调用的
// 超时（ms），供调用方推导 agent 总预算。
func (rt *Router) chatLLMClient(llmCfg *models.AILLMConfig) (llm.LLM, int, error) {
	extraConfig := llmCfg.ExtraConfig
	llmCallTimeout := 120000
	if extraConfig.TimeoutSeconds > 0 {
		llmCallTimeout = extraConfig.TimeoutSeconds * 1000
	}
	client, err := rt.llmClientCache.GetOrCreate(&llm.Config{
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
		// CustomParams 原样透传给 provider，由 provider 决定如何并入请求（OpenAI 兼容
		// 路径把它平铺到 request body 顶层）。
		//
		// chat 路径不再自动注入"关思考"参数（NormalizeThinkingParams 仅存于连接测试
		// probe 路径）：思考是一等公民——思考流接入 thinking 通道、有状态思考协议
		// （Anthropic 思考块回填 / Gemini thoughtSignature）已在 provider 适配层实现。
		// 想关思考的用户在 CustomParams 里按厂商字段显式配置即可。
		ExtraBody: extraConfig.CustomParams,
	})
	return client, llmCallTimeout, err
}

// resolveActionKey 是路由收缩后的确定性 action 解析，纯函数零 LLM。优先级：
//  1. fast    — 创建动词命中 → creation（保住零 LLM 即时弹业务组表单的 UX）；
//  2. import  — 导入现成规则包/模板 → 同样进 creation 前置弹表单。与 fast 分开是因为
//     import 要"列出/有哪些"地浏览包，会被 fast 的 queryVerbs 反信号误挡。
//  3. form    — 上轮以 form_select 收尾（AwaitingForm）且本轮带 action.param
//     （= 表单提交），确定性继承上轮 action；
//  4. default — general_chat 通用 agent（工具全集 + 技能目录自取 + 工具级门）。
//
// 历史上还有 front 分支（首条消息前端显式指定 action）：fe 发送前一律剥掉
// action.key，该通道已死，连同专用 action 一并删除。
func resolveActionKey(content string, paramCount int, prevRoute *models.ConversationRoute) (string, string) {
	switch {
	case chat.HasCreationIntent(content):
		return string(models.ActionKeyCreation), "fast"
	case chat.HasImportIntent(content):
		return string(models.ActionKeyCreation), "import"
	case prevRoute != nil && prevRoute.AwaitingForm && prevRoute.ActionKey != "" && paramCount > 0:
		return prevRoute.ActionKey, "form"
	default:
		return string(models.ActionKeyGeneralChat), "default"
	}
}

// buildAgentInputs 组装 agent 运行的确定性工具参数通道（params）。分层合并，后者覆盖前者：
//  1. user_input — 恒存在，下游（日志、工具参数）可依赖；
//  2. Context 结构化值默认转发（所有 action 生效）— 表单提交/页面上下文里的
//     busi_group_id/datasource_id/team_ids 直达工具层。
//     这是写工具缺参门（tools/form_gate.go）的确定性回填通道：general_chat 路径上
//     缺参表单的提交值必须经此抵达 params，否则只剩提示词文本（ContextDump）一条
//     模型自觉通道，模型不复写 group_id 时同一张表单会再次弹出（表单回环）；
//  3. 身份键（user_id/chat_id/seq_id）— 权限工具与跨轮确认门（update_dashboard 的
//     propose→confirm：确认必须晚于提案轮 SeqID）依赖，恒为 router 权威值。
func buildAgentInputs(chatReq *chat.AIChatRequest, userId int64, chatID string, seqID int64) map[string]string {
	inputs := map[string]string{"user_input": chatReq.UserInput}
	for k, v := range chat.ContextForwardInputs(chatReq) {
		inputs[k] = v
	}
	inputs["user_id"] = fmt.Sprintf("%d", userId)
	inputs["chat_id"] = chatID
	inputs["seq_id"] = fmt.Sprintf("%d", seqID)
	// UI 语言直达工具层：update_* 等工具的确定性确认文案（不经 LLM 转述）按此
	// 选取 zh/en 预制文案（aiagent.LangText），与审批按钮/resume 提示语言一致。
	inputs["lang"] = chatReq.Language
	return inputs
}

// buildMCPOAuthPayload 渲染 mcp_oauth 载荷：尚待用户完成 OAuth 授权的 MCP。id 是
// 前端 POST /mcp-server-oauth/prepare 换取厂商授权页 URL 所需的键。
//
// detected_at 是「本卡片判定其不可用」的时刻。卡片会持久化在历史消息里、每次打开会话
// 都重新渲染，前端据此与 /mcp-server-oauth/status 的 updated_at 比较：只有授权发生在
// 本卡片之后，才把按钮换成「已连接」。不带这个时间戳就只有两种坏结局——要么永远显示
// 按钮（授权完了旧卡片还在喊你授权），要么无脑信 status 而把按钮藏掉（运行时被 provider
// 拒绝、但 token 本地看着仍可用时，用户反而没法自救）。
func buildMCPOAuthPayload(servers []*models.MCPServer) string {
	type item struct {
		Id   int64  `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	items := make([]item, 0, len(servers))
	for _, s := range servers {
		items = append(items, item{Id: s.Id, Name: s.Name, URL: s.URL})
	}
	body, _ := json.Marshal(map[string]interface{}{
		"servers":     items,
		"detected_at": time.Now().Unix(),
	})
	return string(body)
}

// hasFormSelect 报告响应集是否含 form_select 表单（用于 AwaitingForm 路由标记）。
func hasFormSelect(resps []models.AssistantMessageResponse) bool {
	for _, r := range resps {
		if r.ContentType == models.ContentTypeFormSelect {
			return true
		}
	}
	return false
}

// extractImportedAlertRuleCards pulls the per-rule "cards" out of an
// import_alert_rule_template summary, re-marshaling each to a JSON string so it
// matches the Content shape the alert_rule card path expects. Returns nil on
// parse failure or when no rules were created.
func extractImportedAlertRuleCards(obs string) []string {
	var summary struct {
		Cards []json.RawMessage `json:"cards"`
	}
	if err := json.Unmarshal([]byte(obs), &summary); err != nil {
		logger.Warningf("[Assistant] import_alert_rule_template result not parseable for cards: %v", err)
		return nil
	}
	out := make([]string, 0, len(summary.Cards))
	for _, c := range summary.Cards {
		out = append(out, string(c))
	}
	return out
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
// finalText 非空时作为本轮的 assistant 终态写进会话 transcript（resume 路径用：
// 确认/取消的结果文本要让后续轮模型可见）；空 = 纯中断轮，不落 assistant turn。
func (rt *Router) finishHaltedMessage(state *MessageState, streamID string, history []aiagent.ChatMessage, responses []models.AssistantMessageResponse, route *models.ConversationRoute, finalText string) {
	msg := state.Msg()

	if len(responses) > 0 {
		responses[0].StreamID = streamID
		for i := range responses {
			responses[i].IsFinish = true
			responses[i].IsFromAI = true
		}
	}

	// Publish responses BEFORE Finish so stream-only consumers (A2A bridge)
	// see the payload, not just an empty COMPLETED task. SSE 仍由 /detail 渲染，
	// 沿途忽略此帧。
	for _, r := range responses {
		if err := rt.streamBus.PublishResponse(context.Background(), msg.ChatID, streamID, r); err != nil {
			logger.Warningf("[Assistant] PublishResponse on halt chat=%s stream=%s: %v", msg.ChatID, streamID, err)
		}
	}
	_ = rt.streamBus.Finish(context.Background(), msg.ChatID, streamID)

	msg.Response = responses
	msg.IsFinish = true
	msg.CurStep = ""

	// Persist history: the user's turn, plus the deterministic final text when the
	// caller produced one (resume path); a plain halt has no assistant content.
	newHistory := assembleTurnHistory(history, msg.Query.Content, nil, finalText)
	env := aiagent.TranscriptEnvelope{SchemaVersion: aiagent.TranscriptSchemaVersion, Messages: newHistory}
	msg.Extra.HistoryMessages, _ = json.Marshal(env)
	// Carry route state through the halt so the follow-up turn inherits the action.
	msg.Extra.Route = route

	if err := models.AssistantMessageSet(rt.Ctx, *msg); err != nil {
		logger.Errorf("[Assistant] failed to save halted message: %v", err)
	}
	state.Persist(context.Background())
}

// ==================== Stream Segments ====================

// streamSegment 是消费循环按到达顺序累积的展示段。切分规则与 aiagent/a2a/bridge.go
// 的 step 分段、前端流式 reducer 三方同构——同类追加、类型切换开新段、轮边界
// (tool_result) 收口——保证流式视图与持久化视图的块结构一致。
type streamSegment struct {
	kind   string // segmentKindReasoning | segmentKindContent
	text   strings.Builder
	closed bool // 轮边界置位：下一个同类 delta 必须开新段，避免跨轮思考粘连
}

const (
	segmentKindReasoning = "reasoning"
	segmentKindContent   = "content"
)

// segmentAccumulator 聚合一条消息的全部展示段。零值可用。
type segmentAccumulator struct {
	segments []*streamSegment
}

func (a *segmentAccumulator) append(kind, delta string) {
	if n := len(a.segments); n > 0 && a.segments[n-1].kind == kind && !a.segments[n-1].closed {
		a.segments[n-1].text.WriteString(delta)
		return
	}
	a.closeCurrent()
	seg := &streamSegment{kind: kind}
	seg.text.WriteString(delta)
	a.segments = append(a.segments, seg)
}

func (a *segmentAccumulator) closeCurrent() {
	if n := len(a.segments); n > 0 {
		a.segments[n-1].closed = true
	}
}

// assembleSegmentResponses 把流式段转成持久化 Response 块，并在末尾追加权威
// markdown 终答块（fullContent 管线产出：UnwrapJSONEnvelope + general_chat 校验）。
// 纯函数，单测见 router_ai_assistant_test.go。
//
// finalBodyStreamed 为真（正常流式收尾）时，最后一个 content 段是终答的原始
// 流式形态——丢弃它，避免与权威终答块重复；从尾部搜索而非固定取末段，是为了
// 容错"终答后还有尾随思考"的少见 provider 时序。为假（interrupt 确认文案、
// max-iteration partial 等非流式 Done）时全部保留：此时段里只有中间轮的思考与
// 过渡语，终答块另行承载 Done.Content。
//
// 空白段（如轮间 "\n\n" 分隔帧）被过滤。终答块即使为空也保留，维持"至少一个
// markdown 块"的既有不变量。消息级 streamID 锚在第一个块上（前端
// findStreamResponse 取第一个带 stream_id 的块做流匹配）。
func assembleSegmentResponses(segments []*streamSegment, finalMarkdown, streamID string, finalBodyStreamed bool) []models.AssistantMessageResponse {
	if finalBodyStreamed {
		for i := len(segments) - 1; i >= 0; i-- {
			if segments[i].kind == segmentKindContent {
				rest := make([]*streamSegment, 0, len(segments)-1)
				rest = append(rest, segments[:i]...)
				rest = append(rest, segments[i+1:]...)
				segments = rest
				break
			}
		}
	}

	responses := make([]models.AssistantMessageResponse, 0, len(segments)+1)
	for _, seg := range segments {
		text := strings.TrimSpace(seg.text.String())
		if text == "" {
			continue
		}
		ct := models.ContentTypeReasoning
		if seg.kind == segmentKindContent {
			ct = models.ContentTypeMarkdown
		}
		responses = append(responses, models.AssistantMessageResponse{
			ContentType: ct,
			Content:     text,
			IsFinish:    true,
			IsFromAI:    true,
		})
	}

	responses = append(responses, models.AssistantMessageResponse{
		ContentType: models.ContentTypeMarkdown,
		Content:     finalMarkdown,
		IsFinish:    true,
		IsFromAI:    true,
	})

	responses[0].StreamID = streamID
	return responses
}

// assembleTurnHistory builds the persisted conversation transcript for one turn:
// prior history + the user query + this turn's tool-call/observation messages
// (turnMsgs, emitted by the tool loop via StreamTypeTranscript) + a single
// terminal assistant message holding the cleaned final answer (fullContent).
//
// The terminal assistant turn is always (re)built here from fullContent rather
// than taken from turnMsgs, so the next turn's replay sees the clean answer, not
// the raw "Thought:/Final Answer:" scaffolding. A pure function: no I/O, unit-
// testable. Returns a fresh slice (never aliases prev). When fullContent is empty
// (halted turn, or a turn that produced no answer) no terminal turn is appended.
func assembleTurnHistory(prev []aiagent.ChatMessage, query string, turnMsgs []aiagent.ChatMessage, fullContent string) []aiagent.ChatMessage {
	out := make([]aiagent.ChatMessage, 0, len(prev)+len(turnMsgs)+2)
	out = append(out, prev...)
	out = append(out, aiagent.ChatMessage{Role: "user", Content: query})
	out = append(out, turnMsgs...)
	if fullContent != "" {
		out = append(out, aiagent.ChatMessage{Role: "assistant", Content: fullContent})
	}
	return out
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

	if err := rt.CancelAssistantMessageInternal(c, req.ChatID, req.SeqID); err != nil {
		// ErrMessageNotInflight → 404; everything else → 500.
		if errors.Is(err, ErrMessageNotInflight) {
			ginx.Bomb(http.StatusNotFound, "%s", err.Error())
			return
		}
		ginx.Dangerous(err)
		return
	}

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

	clearWriteDeadline(c.Writer)

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
			bombErr(http.StatusInternalServerError, err)
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
