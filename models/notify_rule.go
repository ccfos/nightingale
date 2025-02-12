package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
)

type NotifyRule struct {
	ID int64 `json:"id" gorm:"primarykey"`
	// 基本配置
	Name         string  `json:"name"`           // 名称
	Description  string  `json:"description"`    // 备注
	Enable       bool    `json:"enable"`         // 启用状态
	UserGroupIds []int64 `json:"user_group_ids"` // 告警组ID

	// 通知配置
	NotifyConfigs []NotifyConfig `json:"notify_configs"`
}

type NotifyConfig struct {
	ChannelID  int64       `json:"channel"`  // 通知媒介(如：阿里云短信)
	TemplateID int64       `json:"template"` // 通知模板
	Params     interface{} `json:"params"`   // 通知参数 用户配置

	Severities []int         `json:"severities"`  // 适用级别(一级告警、二级告警、三级告警)
	TimeRanges []TimeRanges  `json:"time_ranges"` // 适用时段
	LabelKeys  []LabelFilter `json:"label_keys"`  // 适用标签
}

type UserInfoParams struct {
	UserIDs      []int64 `json:"user_ids"`
	UserGroupIDs []int64 `json:"user_group_ids"`
}

type CustomParams = map[string]string

type FlashDutyParams struct {
	IDs []int64 `json:"ids"`
}

type TimeRanges struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Week  string `json:"week"`
}

type LabelFilter struct {
	Key   string `json:"key"`
	Op    string `json:"op"` // == != in not in =~ !~
	Value string `json:"value"`
}

var NotifyRuleCache struct {
}

// 创建 NotifyRule
func CreateNotifyRule(c *ctx.Context, rule *NotifyRule) error {
	return DB(c).Create(rule).Error
}

// 读取 NotifyRule
func GetNotifyRule(c *ctx.Context, id int64) (*NotifyRule, error) {
	var rule NotifyRule
	if err := DB(c).First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// 更新 NotifyRule
func UpdateNotifyRule(c *ctx.Context, rule *NotifyRule) error {
	return DB(c).Save(rule).Error
}

// 删除 NotifyRule
func DeleteNotifyRule(c *ctx.Context, id int64) error {
	return DB(c).Delete(&NotifyRule{}, id).Error
}

func NotifyRuleStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=notify_rule")
		return s, err
	}

	session := DB(ctx).Model(&NotifyRule{}).Select("count(*) as total", "max(update_at) as last_updated").Where("enable = ?", true)

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func NotifyRuleGetsAll(ctx *ctx.Context) ([]*NotifyRule, error) {
	if !ctx.IsCenter {
		rules, err := poster.GetByUrls[[]*NotifyRule](ctx, "/v1/n9e/notify-rules")
		return rules, err
	}

	var rules []*NotifyRule
	err := DB(ctx).Where("enable = ?", true).Find(&rules).Error
	if err != nil {
		return nil, err
	}

	return rules, nil
}
