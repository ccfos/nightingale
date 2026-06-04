package models

import "encoding/json"

type AssistantChat struct {
	ChatID     string            `json:"chat_id"`
	Title      string            `json:"title"`
	LastUpdate int64             `json:"last_update"`
	PageFrom   AssistantPageInfo `json:"page_from"`
	UserID     int64             `json:"user_id"`
	IsNew      bool              `json:"is_new"`
}

type AssistantPageType string

const (
	PageTypeDashboards   AssistantPageType = "dashboards"
	PageTypeAlertHistory AssistantPageType = "alert_history"
	PageTypeActiveAlert  AssistantPageType = "active_alert"
	PageTypeExplorer     AssistantPageType = "explorer"
	PageTypeNotifyTpl    AssistantPageType = "notify_tpl"
	PageTypeDatasource   AssistantPageType = "datasource"
	PageTypeAlertRule    AssistantPageType = "alert_rule"
)

// AssistantPageInfo describes which page the user opened the AI assistant from.
// Page identifies the page type (e.g. explorer, dashboards).
// Param carries optional page-level context whose schema varies by page type;
// for example, the explorer page may include datasource_type and datasource_id,
// while other pages may omit it entirely.
type AssistantPageInfo struct {
	Page  AssistantPageType `json:"page"`
	Param json.RawMessage   `json:"param,omitempty"`
}

// ==================== Send Request ====================

type AssistantSendRequest struct {
	ChatID string                `json:"chat_id"`
	Query  AssistantMessageQuery `json:"query"`
}

// ==================== Message ====================

type AssistantMessage struct {
	ChatID          string                     `json:"chat_id"`
	SeqID           int64                      `json:"seq_id"`
	Query           AssistantMessageQuery      `json:"query"`
	Response        []AssistantMessageResponse `json:"response"`
	CurStep         string                     `json:"cur_step"`
	IsFinish        bool                       `json:"is_finish"`
	RecommendAction []AssistantAction          `json:"recommend_action"`
	ErrCode         int                        `json:"err_code"`
	ErrTitle        string                     `json:"err_title"`
	ErrMsg          string                     `json:"err_msg"`
	ExecutedTools   bool                       `json:"executed_tools"`
	Extra           AssistantMessageExtra      `json:"-"` // internal, not exposed to frontend
}

type AssistantMessageExtra struct {
	HistoryMessages []byte `json:"history_messages"` // json(aiagent.TranscriptEnvelope)：结构化会话 transcript

	// Route 是会话路由状态：路由一旦确立即随会话携带，后续轮默认继承，而非每轮
	// 从零重判。一句无信号的跟进（如"确认"）靠它续接上一轮的 action 与编辑目标。
	Route *ConversationRoute `json:"route,omitempty"`

	// Pending 非空 = 本轮以人在环中断收尾，等待用户确认/输入。
	Pending *PendingInterrupt `json:"pending,omitempty"`
}

// PendingInterrupt 是等待用户响应的人在环中断：
// 工具在 propose 腿要求确认后，运行时把"确认后要重放什么"持久化在此。下一轮用户
// 明确确认时由 router 直接确定性重放（零 LLM），拒绝则作废，其余回复回归正常流程。
type PendingInterrupt struct {
	Kind       string            `json:"kind"`        // approval | input
	Tool       string            `json:"tool"`        // 待重放的内置工具名
	ResumeArgs string            `json:"resume_args"` // 重放参数 JSON（工具 propose 腿备好，运行时不解读）
	Params     map[string]string `json:"params"`      // 原轮 AgentRequest.Params（user_id 等；重放时覆盖 chat_id/seq_id）
	Prompt     string            `json:"prompt"`      // 当时给用户看的确认文案（拒绝/重提案时供上下文）
	SeqID      int64             `json:"seq_id"`      // 提案所在轮
}

// ConversationRoute 会话级路由状态，随每条 AssistantMessage 持久化、下一轮加载继承。
type ConversationRoute struct {
	// ActionKey 本轮最终解析出的 action（如 "edit"/"creation"），作为下一轮意图
	// 分类的先验：延续性消息保持不变，仅显式切换话题时改变。
	ActionKey string `json:"action_key,omitempty"`

	// EditTarget 是 edit action 的资源子路由：EditTargetDashboard | EditTargetAlertRule
	// （aiagent/chat 包定义）。无信号的"确认"靠它续接上一轮在编辑的资源，
	// 而不是兜底到告警规则工作流。非 edit 轮为空。
	EditTarget string `json:"edit_target,omitempty"`

	// AwaitingForm 为真 = 本轮以 form_select 表单收尾（preflight 表单或写工具的
	// input 中断），等待用户提交。下一轮带 action.param 的提交据此确定性继承
	// ActionKey（替代 LLM 分类器判"延续"）。
	AwaitingForm bool `json:"awaiting_form,omitempty"`
}

// ==================== Message Query ====================

type AssistantMessageQuery struct {
	Content  string            `json:"content"`
	Action   AssistantAction   `json:"action"`
	PageFrom AssistantPageInfo `json:"page_from"`
}

// ==================== Message Response ====================

type AssistantContentType string

const (
	ContentTypeMarkdown  AssistantContentType = "markdown"
	ContentTypeHint      AssistantContentType = "hint"
	ContentTypeQuery     AssistantContentType = "query"
	ContentTypeReasoning AssistantContentType = "reasoning"
	ContentTypeAlertRule AssistantContentType = "alert_rule"
	ContentTypeDashboard AssistantContentType = "dashboard"
	// ContentTypeFormSelect carries a multi-field form the user must fill in
	// before a halted creation flow can continue. Payload is a creationFormPayload
	// JSON. Frontend renders fields progressively and submits all picks at once.
	ContentTypeFormSelect AssistantContentType = "form_select"
)

type AssistantMessageResponse struct {
	ContentType AssistantContentType `json:"content_type"`
	Content     string               `json:"content"`
	StreamID    string               `json:"stream_id,omitempty"`
	IsFinish    bool                 `json:"is_finish"`
	IsFromAI    bool                 `json:"is_from_ai"`
}

// ==================== Action ====================

type AssistantActionKey string

const (
	ActionKeyQueryGenerator      AssistantActionKey = "query_generator"
	ActionKeyGeneralChat         AssistantActionKey = "general_chat"
	ActionKeyCreation            AssistantActionKey = "creation"
	ActionKeyEdit                AssistantActionKey = "edit"
	ActionKeyTroubleshooting     AssistantActionKey = "troubleshooting"
	ActionKeyNotifyTemplate      AssistantActionKey = "notify_template_generator"
	ActionKeyNotifyChannel       AssistantActionKey = "notify_channel_copilot"
	ActionKeyDatasourceDiagnose  AssistantActionKey = "datasource_diagnose"
	ActionKeyHostHealthDiagnose  AssistantActionKey = "host_health_diagnose"
	ActionKeyHostOnboardDiagnose AssistantActionKey = "host_onboard_diagnose"
	ActionKeyTaskTplCopilot      AssistantActionKey = "task_tpl_copilot"
	ActionKeyAutoHealRecommend   AssistantActionKey = "auto_heal_recommend"
	ActionKeyAgentDeployGuide    AssistantActionKey = "agent_deploy_guide"
	ActionKeyDocQA               AssistantActionKey = "doc_qa"
)

type AssistantAction struct {
	Content string                 `json:"content"`
	Key     AssistantActionKey     `json:"key"`
	Param   map[string]interface{} `json:"param,omitempty"`
}

// ==================== Message Status ====================

type AssistantMessageStatus int

const (
	MessageStatusNone   AssistantMessageStatus = 0
	MessageStatusCancel AssistantMessageStatus = -2
)

// ==================== Lock Key ====================

func AssistantChatLockKey(chatID string) string {
	return "ai_assistant_chat_lock:" + chatID
}
