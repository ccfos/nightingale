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
	HistoryMessages []byte `json:"history_messages"` // json([]ChatMessage)
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
	ActionKeyQueryGenerator     AssistantActionKey = "query_generator"
	ActionKeyGeneralChat        AssistantActionKey = "general_chat"
	ActionKeyAlertQuery         AssistantActionKey = "alert_query"
	ActionKeyResourceQuery      AssistantActionKey = "resource_query"
	ActionKeyCreation           AssistantActionKey = "creation"
	ActionKeyTroubleshooting    AssistantActionKey = "troubleshooting"
	ActionKeyNotifyTemplate     AssistantActionKey = "notify_template_generator"
	ActionKeyDatasourceDiagnose AssistantActionKey = "datasource_diagnose"
)

type AssistantAction struct {
	Content string               `json:"content"`
	Key     AssistantActionKey   `json:"key"`
	Param   AssistantActionParam `json:"param"`
}

type AssistantActionParam struct {
	DatasourceType string  `json:"datasource_type,omitempty"`
	DatasourceID   int64   `json:"datasource_id,omitempty"`
	DatabaseName   string  `json:"database_name,omitempty"`
	TableName      string  `json:"table_name,omitempty"`
	BusiGroupID    int64   `json:"busi_group_id,omitempty"`
	TeamIDs        []int64 `json:"team_ids,omitempty"`
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
