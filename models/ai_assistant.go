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

// ConversationRoute 会话级路由状态，随每条 AssistantMessage 持久化、下一轮加载读取。
type ConversationRoute struct {
	// ActionKey 本轮最终解析出的 action（如 "creation"/"general_chat"）。
	// 仅在表单提交轮（上轮 AwaitingForm + 本轮带 action.param）被确定性继承；
	// 普通延续轮不继承，由 resolveActionKey 重新解析。
	ActionKey string `json:"action_key,omitempty"`

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

// IsStructuredPayload reports whether Content carries a machine-readable JSON
// payload (rendered as a card/widget) rather than human-readable text. The A2A
// bridge tags only structured parts with n9e_content_type metadata — clients
// that see the tag are entitled to json.Unmarshal the part body, so tagging a
// plain-text type (markdown/hint) here would crash them on the first Chinese
// character. New JSON card types MUST be added to this whitelist.
func (t AssistantContentType) IsStructuredPayload() bool {
	switch t {
	case ContentTypeAlertRule, ContentTypeDashboard, ContentTypeFormSelect:
		return true
	}
	return false
}

type AssistantMessageResponse struct {
	ContentType AssistantContentType `json:"content_type"`
	Content     string               `json:"content"`
	StreamID    string               `json:"stream_id,omitempty"`
	IsFinish    bool                 `json:"is_finish"`
	IsFromAI    bool                 `json:"is_from_ai"`
}

// ==================== Action ====================

type AssistantActionKey string

// 仅存对话路径实际可达的两个 action（路由收缩 + fe 剥 action.key 后，
// 历史上的专用 action 常量已随 chat.registry 条目一并删除；
// 未知 key 由 router 兜底降级到 general_chat）。
const (
	ActionKeyGeneralChat AssistantActionKey = "general_chat"
	ActionKeyCreation    AssistantActionKey = "creation"
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
