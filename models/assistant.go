package models

// assistant.go — Proto structs for AI assistant, aligned with fc-model.
// These structs are serialized as JSON into the DB `data` column (gzip encoded).
// They are also the API response shapes — no separate "view" layer needed.

// ==================== Chat ====================

type AssistantChat struct {
	ChatID          string          `json:"chat_id"`
	Title           string          `json:"title"`
	LastUpdate      int64           `json:"last_update"`
	PageFrom        AssistantPageInfo `json:"page_from"`
	RecommendAction []AssistantAction `json:"recommend_action"`
	UserID          int64           `json:"user_id"`
	IsNew           bool            `json:"is_new"`
}

// ==================== Page Info ====================

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
	WorkspaceID    int64                          `json:"workspace_id,omitempty"`
	Dashboard      *AssistantPageParamDashboard   `json:"dashboard,omitempty"`
	DatasourceType string                         `json:"datasource_type,omitempty"`
	DatasourceID   int64                          `json:"datasource_id,omitempty"`
}

type AssistantPageParamDashboard struct {
	ID    any              `json:"id"`
	Var   map[string][]any `json:"var,omitempty"`
	Start int64            `json:"start,omitempty"`
	End   int64            `json:"end,omitempty"`
}

// ==================== Message ====================

type AssistantMessage struct {
	AssistantMessageMeta
	ModelID         int64                      `json:"model_id"`
	Query           AssistantMessageQuery      `json:"query"`
	Response        []AssistantMessageResponse `json:"response"`
	CurStep         string                     `json:"cur_step"`
	IsFinish        bool                       `json:"is_finish"`
	Feedback        AssistantMessageFeedback   `json:"feedback"`
	RecommendAction []AssistantAction          `json:"recommend_action"`
	ErrCode         int                        `json:"err_code"`
	ErrTitle        string                     `json:"err_title"`
	ErrMsg          string                     `json:"err_msg"`
	ExecutedTools   bool                       `json:"executed_tools"`
	Extra           AssistantMessageExtra      `json:"-"` // internal, not exposed to frontend
}

type AssistantMessageMeta struct {
	ChatID string `json:"chat_id"`
	SeqID  int64  `json:"seq_id"`
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
	HintText    string               `json:"hint_text,omitempty"`
	StreamID    string               `json:"stream_id,omitempty"`
	IsFinish    bool                 `json:"is_finish"`
	Param       any                  `json:"param,omitempty"`
	IsFromAI    bool                 `json:"is_from_ai"`
}

// ==================== Action ====================

type AssistantActionKey string

const (
	ActionKeyQueryGenerator AssistantActionKey = "query_generator"
)

type AssistantAction struct {
	Content string             `json:"content"`
	Key     AssistantActionKey `json:"key"`
	Param   AssistantActionParam `json:"param"`
}

type AssistantActionParam struct {
	DatasourceType string `json:"datasource_type,omitempty"`
	DatasourceID   int64  `json:"datasource_id,omitempty"`
	DatabaseName   string `json:"database_name,omitempty"`
	TableName      string `json:"table_name,omitempty"`
}

// ==================== Feedback / Status ====================

type AssistantMessageStatus int

const (
	MessageStatusNone    AssistantMessageStatus = 0
	MessageStatusLike    AssistantMessageStatus = 1
	MessageStatusDislike AssistantMessageStatus = -1
	MessageStatusCancel  AssistantMessageStatus = -2
)

type AssistantMessageFeedback struct {
	AssistantMessageMeta
	Status AssistantMessageStatus `json:"status"`
}

// ==================== Lock Key ====================

func AssistantChatLockKey(chatID string) string {
	return "ai_assistant_chat_lock:" + chatID
}
