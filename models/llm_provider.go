package models

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type LLMProvider struct {
	Id          int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string `json:"name"`
	APIType     string `json:"api_type"`
	APIURL      string `json:"api_url"`
	APIKey      string `json:"api_key"`
	Model       string `json:"model"`
	IsDefault   int    `json:"is_default"`
	Enabled     int    `json:"enabled"`
	ExtraConfig string `json:"extra_config"`
	CreatedAt   int64  `json:"created_at"`
	CreatedBy   string `json:"created_by"`
	UpdatedAt   int64  `json:"updated_at"`
	UpdatedBy   string `json:"updated_by"`
}

func (p *LLMProvider) TableName() string {
	return "llm_provider"
}

func (p *LLMProvider) Verify() error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if p.APIType == "" {
		return fmt.Errorf("api_type is required")
	}
	if p.APIURL == "" {
		return fmt.Errorf("api_url is required")
	}
	if p.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}
	if p.Model == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

func LLMProviderGets(c *ctx.Context) ([]*LLMProvider, error) {
	var lst []*LLMProvider
	err := DB(c).Order("id").Find(&lst).Error
	return lst, err
}

func LLMProviderGet(c *ctx.Context, where string, args ...interface{}) (*LLMProvider, error) {
	var obj LLMProvider
	err := DB(c).Where(where, args...).First(&obj).Error
	if err != nil {
		if err.Error() == "record not found" {
			return nil, nil
		}
		return nil, err
	}
	return &obj, nil
}

func LLMProviderGetById(c *ctx.Context, id int64) (*LLMProvider, error) {
	return LLMProviderGet(c, "id = ?", id)
}

func (p *LLMProvider) Create(c *ctx.Context) error {
	now := time.Now().Unix()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Enabled == 0 {
		p.Enabled = 1
	}

	// If setting as default, clear other defaults
	if p.IsDefault == 1 {
		DB(c).Model(&LLMProvider{}).Where("is_default = 1").Update("is_default", 0)
	}

	return Insert(c, p)
}

func (p *LLMProvider) Update(c *ctx.Context, ref LLMProvider) error {
	ref.UpdatedAt = time.Now().Unix()

	// If setting as default, clear other defaults
	if ref.IsDefault == 1 {
		DB(c).Model(&LLMProvider{}).Where("id <> ? and is_default = 1", p.Id).Update("is_default", 0)
	}

	return DB(c).Model(p).Select("name", "api_type", "api_url", "api_key", "model",
		"is_default", "enabled", "extra_config", "updated_at", "updated_by").Updates(ref).Error
}

func (p *LLMProvider) Delete(c *ctx.Context) error {
	return DB(c).Where("id = ?", p.Id).Delete(&LLMProvider{}).Error
}

func LLMProviderGetDefault(c *ctx.Context) (*LLMProvider, error) {
	return LLMProviderGet(c, "is_default = 1 and enabled = 1")
}

func LLMProviderStatistics(c *ctx.Context) (*Statistics, error) {
	return StatisticsGet(c, &LLMProvider{})
}
