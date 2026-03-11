package models

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type AIAgent struct {
	Id           int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	UseCase      string  `json:"use_case"`
	LLMConfigId  int64   `json:"llm_config_id"`
	SkillIds     []int64 `json:"skill_ids" gorm:"serializer:json"`
	MCPServerIds []int64 `json:"mcp_server_ids" gorm:"serializer:json"`
	Enabled      int     `json:"enabled"`
	CreatedAt    int64   `json:"created_at"`
	CreatedBy    string  `json:"created_by"`
	UpdatedAt    int64   `json:"updated_at"`
	UpdatedBy    string  `json:"updated_by"`

	// Runtime: resolved from LLMConfigId, not stored in DB
	LLMConfig *AILLMConfig `json:"llm_config,omitempty" gorm:"-"`
}

func (a *AIAgent) TableName() string {
	return "ai_agent"
}

func (a *AIAgent) Verify() error {
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
		if err.Error() == "record not found" {
			return nil, nil
		}
		return nil, err
	}
	return &obj, nil
}

func AIAgentGetById(c *ctx.Context, id int64) (*AIAgent, error) {
	return AIAgentGet(c, "id = ?", id)
}

func (a *AIAgent) Create(c *ctx.Context, username string) error {
	now := time.Now().Unix()
	a.CreatedAt = now
	a.UpdatedAt = now
	a.CreatedBy = username
	a.UpdatedBy = username
	if a.Enabled == 0 {
		a.Enabled = 1
	}

	return Insert(c, a)
}

func (a *AIAgent) Update(c *ctx.Context, username string, data AIAgent) error {
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
	return AIAgentGet(c, "use_case = ? and enabled = 1", useCase)
}

func AIAgentStatistics(c *ctx.Context) (*Statistics, error) {
	return StatisticsGet(c, &AIAgent{})
}
