package models

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type AILLMConfig struct {
	Id          int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string `json:"name"`
	Description string `json:"description"`
	APIType     string `json:"api_type"`
	APIURL      string `json:"api_url"`
	APIKey      string `json:"api_key"`
	Model       string `json:"model"`
	ExtraConfig string `json:"extra_config"`
	Enabled     int    `json:"enabled"`
	CreatedAt   int64  `json:"created_at"`
	CreatedBy   string `json:"created_by"`
	UpdatedAt   int64  `json:"updated_at"`
	UpdatedBy   string `json:"updated_by"`
}

func (a *AILLMConfig) TableName() string {
	return "ai_llm_config"
}

func (a *AILLMConfig) Verify() error {
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

func AILLMConfigGets(c *ctx.Context) ([]*AILLMConfig, error) {
	var lst []*AILLMConfig
	err := DB(c).Order("id").Find(&lst).Error
	return lst, err
}

func AILLMConfigGet(c *ctx.Context, where string, args ...interface{}) (*AILLMConfig, error) {
	var obj AILLMConfig
	err := DB(c).Where(where, args...).First(&obj).Error
	if err != nil {
		if err.Error() == "record not found" {
			return nil, nil
		}
		return nil, err
	}
	return &obj, nil
}

func AILLMConfigGetById(c *ctx.Context, id int64) (*AILLMConfig, error) {
	return AILLMConfigGet(c, "id = ?", id)
}

func AILLMConfigGetEnabled(c *ctx.Context) ([]*AILLMConfig, error) {
	var lst []*AILLMConfig
	err := DB(c).Where("enabled = 1").Order("id").Find(&lst).Error
	return lst, err
}

func (a *AILLMConfig) Create(c *ctx.Context, username string) error {
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

func (a *AILLMConfig) Update(c *ctx.Context, username string, data AILLMConfig) error {
	data.UpdatedAt = time.Now().Unix()
	data.UpdatedBy = username

	// If api_key is empty, keep the original
	if data.APIKey == "" {
		data.APIKey = a.APIKey
	}

	return DB(c).Model(a).Select("name", "description", "api_type", "api_url", "api_key", "model",
		"extra_config", "enabled", "updated_at", "updated_by").Updates(data).Error
}

func (a *AILLMConfig) Delete(c *ctx.Context) error {
	return DB(c).Where("id = ?", a.Id).Delete(&AILLMConfig{}).Error
}
