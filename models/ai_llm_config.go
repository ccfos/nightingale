package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"gorm.io/gorm"
)

type LLMExtraConfig struct {
	TimeoutSeconds int                `json:"timeout_seconds,omitempty"`
	SkipTLSVerify  bool               `json:"skip_tls_verify,omitempty"`
	Proxy          string             `json:"proxy,omitempty"`
	CustomHeaders  map[string]string  `json:"custom_headers,omitempty"`
	CustomParams   map[string]any     `json:"custom_params,omitempty"`
	Temperature    *float64           `json:"temperature,omitempty"`
	MaxTokens      *int               `json:"max_tokens,omitempty"`
	ContextLength  *int               `json:"context_length,omitempty"`
}

type AILLMConfig struct {
	Id          int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	APIType     string         `json:"api_type"`
	APIURL      string         `json:"api_url"`
	APIKey      string         `json:"api_key"`
	Model       string         `json:"model"`
	ExtraConfig LLMExtraConfig `json:"extra_config" gorm:"serializer:json"`
	Enabled     bool           `json:"enabled"`
	IsDefault   bool           `json:"is_default" gorm:"column:is_default;type:boolean;default:false"`
	CreatedAt   int64          `json:"created_at"`
	CreatedBy   string         `json:"created_by"`
	UpdatedAt   int64          `json:"updated_at"`
	UpdatedBy   string         `json:"updated_by"`
}

func (a *AILLMConfig) TableName() string {
	return "ai_llm_config"
}

func (a *AILLMConfig) Verify() error {
	a.Name = strings.TrimSpace(a.Name)
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

// MaskAPIKey masks api_key for display: keep first/last 4 chars, mask the middle.
// Keys of 8 chars or shorter are fully masked.
func (a *AILLMConfig) MaskAPIKey() {
	a.APIKey = maskAPIKey(a.APIKey)
}

func maskAPIKey(key string) string {
	n := len(key)
	if n == 0 {
		return ""
	}
	if n <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[n-4:]
}

// IsMaskedAPIKey reports whether `raw` is the mask produced by MaskAPIKey for
// `stored`. Used by update handlers to detect the case where the frontend
// round-trips the masked key it received from a GET — in which case we must
// keep the real stored key instead of overwriting it with the mask.
func IsMaskedAPIKey(raw, stored string) bool {
	if raw == "" || stored == "" {
		return false
	}
	return raw == maskAPIKey(stored)
}

func AILLMConfigGet(c *ctx.Context, where string, args ...interface{}) (*AILLMConfig, error) {
	var obj AILLMConfig
	err := DB(c).Where(where, args...).First(&obj).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &obj, nil
}

func AILLMConfigGetById(c *ctx.Context, id int64) (*AILLMConfig, error) {
	return AILLMConfigGet(c, "id = ?", id)
}

func AILLMConfigGetByIds(c *ctx.Context, ids []int64) ([]*AILLMConfig, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var lst []*AILLMConfig
	err := DB(c).Where("id in ?", ids).Find(&lst).Error
	return lst, err
}

func AILLMConfigGetByName(c *ctx.Context, name string) (*AILLMConfig, error) {
	return AILLMConfigGet(c, "name = ?", name)
}

func AILLMConfigGetEnabled(c *ctx.Context) ([]*AILLMConfig, error) {
	var lst []*AILLMConfig
	err := DB(c).Where("enabled = ?", true).Order("id").Find(&lst).Error
	return lst, err
}

// AILLMConfigPickDefault returns the LLM config marked is_default=true (and
// enabled). Used by auto-wired consumers such as the default chat agent when
// it has no LLMConfigId bound. Returns (nil, nil) when no such row exists.
func AILLMConfigPickDefault(c *ctx.Context) (*AILLMConfig, error) {
	var obj AILLMConfig
	err := DB(c).Where("is_default = ? AND enabled = ?", true, true).First(&obj).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &obj, nil
}

func (a *AILLMConfig) Create(c *ctx.Context, username string) error {
	now := time.Now().Unix()
	a.CreatedAt = now
	a.UpdatedAt = now
	a.CreatedBy = username
	a.UpdatedBy = username

	return DB(c).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&AILLMConfig{}).Where("name = ?", a.Name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("ai llm config name %s already exists", a.Name)
		}
		if a.IsDefault {
			if err := tx.Model(&AILLMConfig{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(a).Error
	})
}

func (a *AILLMConfig) Update(c *ctx.Context, username string, data AILLMConfig) error {
	data.UpdatedAt = time.Now().Unix()
	data.UpdatedBy = username

	return DB(c).Transaction(func(tx *gorm.DB) error {
		if data.Name != a.Name {
			var count int64
			if err := tx.Model(&AILLMConfig{}).Where("name = ?", data.Name).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("ai llm config name %s already exists", data.Name)
			}
		}
		if data.IsDefault {
			if err := tx.Model(&AILLMConfig{}).Where("is_default = ? AND id != ?", true, a.Id).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Model(a).Select("name", "description", "api_type", "api_url", "api_key", "model",
			"extra_config", "enabled", "is_default", "updated_at", "updated_by").Updates(data).Error
	})
}

func (a *AILLMConfig) Delete(c *ctx.Context) error {
	return DB(c).Where("id = ?", a.Id).Delete(&AILLMConfig{}).Error
}
