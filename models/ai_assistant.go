package models

type AssistantChat struct {
	ChatID          string            `json:"chat_id"`
	Title           string            `json:"title"`
	LastUpdate      int64             `json:"last_update"`
	PageFrom        AssistantPageInfo `json:"page_from"`
	RecommendAction []AssistantAction `json:"recommend_action"`
	UserID          int64             `json:"user_id"`
	IsNew           bool              `json:"is_new"`
}

type AssistantPageType string

const (
	PageTypeDashboards   AssistantPageType = "dashboards"
	PageTypeAlertHistory AssistantPageType = "alert_history"
	PageTypeActiveAlert  AssistantPageType = "active_alert"
	PageTypeExplorer     AssistantPageType = "explorer"
)

type AssistantPageInfo struct {
	Page  AssistantPageType      `json:"page"`
	Param AssistantPageInfoParam `json:"param"`
}

type AssistantPageInfoParam struct {
	DatasourceType string `json:"datasource_type,omitempty"`
	DatasourceID   int64  `json:"datasource_id,omitempty"`
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
	ContentTypeMarkdown AssistantContentType = "markdown"
	ContentTypeHint     AssistantContentType = "hint"
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
	ActionKeyQueryGenerator AssistantActionKey = "query_generator"
)

type AssistantAction struct {
	Content string               `json:"content"`
	Key     AssistantActionKey   `json:"key"`
	Param   AssistantActionParam `json:"param"`
}

type AssistantActionParam struct {
	DatasourceType string `json:"datasource_type,omitempty"`
	DatasourceID   int64  `json:"datasource_id,omitempty"`
	DatabaseName   string `json:"database_name,omitempty"`
	TableName      string `json:"table_name,omitempty"`
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
