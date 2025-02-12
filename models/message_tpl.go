package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
)

// MessageTemplate 消息模板结构
type MessageTemplate struct {
	ID           int64             `json:"id" gorm:"primarykey"`
	Name         string            `json:"name"`    // 模板名称
	Ident        string            `json:"ident"`   // 模板标识
	Content      map[string]string `json:"content"` // 模板内容
	UserGroupIds []int64           `json:"user_group_ids"`
}

type HTTPConfig struct {
	Type     string `json:"type"`
	IsGlobal bool   `json:"is_global"`
	Name     string `json:"name"`
	Ident    string `json:"ident"`
	Note     string `json:"note"` // 备注

	Enabled     bool              `json:"enabled"`     // 是否启用
	URL         string            `json:"url"`         // 回调URL
	Method      string            `json:"method"`      // HTTP方法
	Headers     map[string]string `json:"headers"`     // 请求头
	Timeout     int               `json:"timeout"`     // 超时时间(毫秒)
	Concurrency int               `json:"concurrency"` // 并发度
	RetryTimes  int               `json:"retryTimes"`  // 重试次数
	RetryDelay  int               `json:"retryDelay"`  // 重试间隔(毫秒)
	SkipVerify  bool              `json:"skipVerify"`  // 跳过SSL校验
	Proxy       string            `json:"proxy"`       // 代理地址

	// 请求参数配置
	EnableParams bool              `json:"enableParams"` // 启用Params参数
	Params       map[string]string `json:"params"`       // URL参数

	// 请求体配置
	EnableBody bool   `json:"enableBody"` // 启用Body
	Body       string `json:"body"`       // 请求体内容
}

func MessageTemplateStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=message_template")
		return s, err
	}

	session := DB(ctx).Model(&MessageTemplate{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func MessageTemplateGetsAll(ctx *ctx.Context) ([]*MessageTemplate, error) {
	if !ctx.IsCenter {
		templates, err := poster.GetByUrls[[]*MessageTemplate](ctx, "/v1/n9e/message-templates")
		return templates, err
	}

	var templates []*MessageTemplate
	err := DB(ctx).Find(&templates).Error
	if err != nil {
		return nil, err
	}

	return templates, nil
}
