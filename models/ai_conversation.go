package models

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type AIConversation struct {
	Id        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Title     string `json:"title"`
	UserId    int64  `json:"user_id"`
	Context   string `json:"context" gorm:"type:text"` // JSON, page-specific context (datasource, alert rule, etc.)
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

func (c *AIConversation) TableName() string {
	return "ai_conversation"
}

func AIConversationGetsByUserId(c *ctx.Context, userId int64) ([]*AIConversation, error) {
	var lst []*AIConversation
	err := DB(c).Where("user_id = ?", userId).Order("updated_at desc").Find(&lst).Error
	return lst, err
}

func AIConversationGetById(c *ctx.Context, id int64) (*AIConversation, error) {
	var obj AIConversation
	err := DB(c).Where("id = ?", id).First(&obj).Error
	if err != nil {
		if err.Error() == "record not found" {
			return nil, nil
		}
		return nil, err
	}
	return &obj, nil
}

func (a *AIConversation) Create(c *ctx.Context) error {
	now := time.Now().Unix()
	a.CreatedAt = now
	a.UpdatedAt = now
	return Insert(c, a)
}

func (a *AIConversation) Update(c *ctx.Context, title string) error {
	a.Title = title
	a.UpdatedAt = time.Now().Unix()
	return DB(c).Model(a).Select("title", "updated_at").Updates(a).Error
}

func (a *AIConversation) UpdateTime(c *ctx.Context) error {
	a.UpdatedAt = time.Now().Unix()
	return DB(c).Model(a).Select("updated_at").Updates(a).Error
}

func (a *AIConversation) Delete(c *ctx.Context) error {
	// Cascade delete messages
	if err := DB(c).Where("conversation_id = ?", a.Id).Delete(&AIConversationMessage{}).Error; err != nil {
		return err
	}
	return DB(c).Where("id = ?", a.Id).Delete(&AIConversation{}).Error
}

func (a *AIConversation) Verify() error {
	if a.UserId == 0 {
		return fmt.Errorf("user_id is required")
	}
	return nil
}

type AIConversationMessage struct {
	Id             int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	ConversationId int64  `json:"conversation_id"`
	Role           string `json:"role"`
	Content        string `json:"content" gorm:"type:text"`
	Thinking       string `json:"thinking" gorm:"type:text"`
	ToolCalls      string `json:"tool_calls" gorm:"type:text"`
	Query          string `json:"query" gorm:"type:text"`
	Explanation    string `json:"explanation" gorm:"type:text"`
	Error          string `json:"error" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at"`
}

func (m *AIConversationMessage) TableName() string {
	return "ai_conversation_message"
}

func AIConversationMessageGetsByConversationId(c *ctx.Context, conversationId int64) ([]*AIConversationMessage, error) {
	var lst []*AIConversationMessage
	err := DB(c).Where("conversation_id = ?", conversationId).Order("id asc").Find(&lst).Error
	return lst, err
}

func (m *AIConversationMessage) Create(c *ctx.Context) error {
	m.CreatedAt = time.Now().Unix()
	return Insert(c, m)
}

func AIConversationMessageDeleteByConversationId(c *ctx.Context, conversationId int64) error {
	return DB(c).Where("conversation_id = ?", conversationId).Delete(&AIConversationMessage{}).Error
}
