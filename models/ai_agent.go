package models

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type AIAgent struct {
	Id           int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	APIType      string `json:"api_type"`
	APIURL       string `json:"api_url"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	ExtraConfig  string `json:"extra_config"`
	SkillIds     string `json:"skill_ids"`
	MCPServerIds string `json:"mcp_server_ids"`
	IMConfig     string `json:"im_config"`
	IsDefault    int    `json:"is_default"`
	Enabled      int    `json:"enabled"`
	CreatedAt    int64  `json:"created_at"`
	CreatedBy    string `json:"created_by"`
	UpdatedAt    int64  `json:"updated_at"`
	UpdatedBy    string `json:"updated_by"`
}

func (a *AIAgent) TableName() string {
	return "ai_agent"
}

func (a *AIAgent) Verify() error {
	if a.Name == "" {
		return fmt.Errorf("name is required")
	}
	if a.APIType == "" {
		return fmt.Errorf("api_type is required")
	}
	if a.APIURL == "" {
		return fmt.Errorf("api_url is required")
	}
	if a.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}
	if a.Model == "" {
		return fmt.Errorf("model is required")
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

	// If setting as default, clear other defaults
	if a.IsDefault == 1 {
		DB(c).Model(&AIAgent{}).Where("is_default = 1").Update("is_default", 0)
	}

	return Insert(c, a)
}

func (a *AIAgent) Update(c *ctx.Context, username string, data AIAgent) error {
	data.UpdatedAt = time.Now().Unix()
	data.UpdatedBy = username

	// If setting as default, clear other defaults
	if data.IsDefault == 1 {
		DB(c).Model(&AIAgent{}).Where("id <> ? and is_default = 1", a.Id).Update("is_default", 0)
	}

	// If api_key is empty, keep the original
	if data.APIKey == "" {
		data.APIKey = a.APIKey
	}

	return DB(c).Model(a).Select("name", "description", "api_type", "api_url", "api_key", "model",
		"extra_config", "skill_ids", "mcp_server_ids", "im_config",
		"is_default", "enabled", "updated_at", "updated_by").Updates(data).Error
}

func (a *AIAgent) Delete(c *ctx.Context) error {
	return DB(c).Where("id = ?", a.Id).Delete(&AIAgent{}).Error
}

func AIAgentGetDefault(c *ctx.Context) (*AIAgent, error) {
	return AIAgentGet(c, "is_default = 1 and enabled = 1")
}

func AIAgentStatistics(c *ctx.Context) (*Statistics, error) {
	return StatisticsGet(c, &AIAgent{})
}
