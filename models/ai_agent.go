package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

type AIAgent struct {
	Id           int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	UseCase      string  `json:"use_case"`
	LLMConfigId  int64   `json:"llm_config_id"`
	SkillIds     []int64 `json:"skill_ids,omitempty" gorm:"serializer:json"`
	MCPServerIds []int64 `json:"mcp_server_ids,omitempty" gorm:"serializer:json"`
	Enabled      bool    `json:"enabled"`
	CreatedAt    int64   `json:"created_at"`
	CreatedBy    string  `json:"created_by"`
	UpdatedAt    int64   `json:"updated_at"`
	UpdatedBy    string  `json:"updated_by"`

	LLMConfigName string `json:"llm_config_name" gorm:"-"`
}

func (a *AIAgent) TableName() string {
	return "ai_agent"
}

func (a *AIAgent) Verify() error {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return fmt.Errorf("name is required")
	}
	if a.LLMConfigId <= 0 {
		return fmt.Errorf("llm_config_id is required")
	}
	return nil
}

func AIAgentGets(c *ctx.Context) ([]*AIAgent, error) {
	var lst []*AIAgent
	err := DB(c).Order("id").Find(&lst).Error
	return lst, err
}

func AIAgentGet(c *ctx.Context, where string, args ...interface{}) (*AIAgent, error) {
	var obj AIAgent
	err := DB(c).Where(where, args...).First(&obj).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &obj, nil
}

func AIAgentGetById(c *ctx.Context, id int64) (*AIAgent, error) {
	return AIAgentGet(c, "id = ?", id)
}

func AIAgentGetByName(c *ctx.Context, name string) (*AIAgent, error) {
	return AIAgentGet(c, "name = ?", name)
}

func (a *AIAgent) Create(c *ctx.Context, username string) error {
	exist, err := AIAgentGetByName(c, a.Name)
	if err != nil {
		return err
	}
	if exist != nil {
		return fmt.Errorf("ai agent name %s already exists", a.Name)
	}

	now := time.Now().Unix()
	a.CreatedAt = now
	a.UpdatedAt = now
	a.CreatedBy = username
	a.UpdatedBy = username
	return Insert(c, a)
}

func (a *AIAgent) Update(c *ctx.Context, username string, data AIAgent) error {
	if data.Name != a.Name {
		exist, err := AIAgentGetByName(c, data.Name)
		if err != nil {
			return err
		}
		if exist != nil {
			return fmt.Errorf("ai agent name %s already exists", data.Name)
		}
	}

	data.UpdatedAt = time.Now().Unix()
	data.UpdatedBy = username

	return DB(c).Model(a).Select("name", "description", "use_case", "llm_config_id",
		"skill_ids", "mcp_server_ids",
		"enabled", "updated_at", "updated_by").Updates(data).Error
}

func (a *AIAgent) Delete(c *ctx.Context) error {
	return DB(c).Where("id = ?", a.Id).Delete(&AIAgent{}).Error
}

func AIAgentGetByUseCase(c *ctx.Context, useCase string) (*AIAgent, error) {
	return AIAgentGet(c, "use_case = ? and enabled = ?", useCase, true)
}

func AIAgentStatistics(c *ctx.Context) (*Statistics, error) {
	return StatisticsGet(c, &AIAgent{})
}

// InitDefaultAIAgent ensures at least one agent exists so aichat works even
// when the admin UI no longer exposes agent management. If the ai_agent table
// is empty, create a default use_case=chat agent with LLMConfigId=0 — the
// runtime resolves it to the default LLM via AILLMConfigPickDefault when the
// chat turn actually runs. Idempotent.
func InitDefaultAIAgent(c *ctx.Context) {
	var count int64
	if err := DB(c).Model(&AIAgent{}).Count(&count).Error; err != nil {
		logger.Warningf("InitDefaultAIAgent count failed: %v", err)
		return
	}
	if count > 0 {
		return
	}

	now := time.Now().Unix()
	agent := &AIAgent{
		Name:        "default-chat-agent",
		Description: "auto-created default chat agent; uses the default LLM config at runtime",
		UseCase:     "chat",
		LLMConfigId: 0,
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "system",
		UpdatedBy:   "system",
	}
	if err := Insert(c, agent); err != nil {
		logger.Warningf("InitDefaultAIAgent insert failed: %v", err)
	}
}
