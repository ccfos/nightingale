package models

import (
	"errors"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
)

type NotifyRule struct {
	ID           int64   `json:"id" gorm:"primarykey"`
	Name         string  `json:"name"`                                  // 名称
	Description  string  `json:"description"`                           // 备注
	Enable       bool    `json:"enable"`                                // 启用状态
	UserGroupIds []int64 `json:"user_group_ids" gorm:"serializer:json"` // 告警组ID

	// 通知配置
	NotifyConfigs []NotifyConfig `json:"notify_configs" gorm:"serializer:json"`

	CreateAt int64  `json:"create_at"`
	CreateBy string `json:"create_by"`
	UpdateAt int64  `json:"update_at"`
	UpdateBy string `json:"update_by"`
}

type NotifyConfig struct {
	ChannelID  int64       `json:"channel_id"`  // 通知媒介(如：阿里云短信)
	TemplateID int64       `json:"template_id"` // 通知模板
	Params     interface{} `json:"params"`      // 通知参数

	UserInfoParams  UserInfoParams  `json:"user_info_params"`  // 通知对象
	CustomParams    CustomParams    `json:"custom_params"`     // 自定义参数
	FlashDutyParams FlashDutyParams `json:"flash_duty_params"` // flash_duty 参数

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

func (r *NotifyRule) Verify() error {
	if r.Name == "" {
		return errors.New("name cannot be empty")
	}

	if len(r.UserGroupIds) == 0 {
		return errors.New("user group ids cannot be empty")
	}

	if len(r.NotifyConfigs) == 0 {
		return errors.New("notify configs cannot be empty")
	}

	for _, config := range r.NotifyConfigs {
		if err := config.Verify(); err != nil {
			return err
		}
	}

	return nil
}

func (c *NotifyConfig) Verify() error {
	if c.ChannelID <= 0 {
		return errors.New("invalid channel id")
	}

	if len(c.Severities) == 0 {
		return errors.New("severities cannot be empty")
	}
	for _, severity := range c.Severities {
		if severity < 1 || severity > 3 {
			return errors.New("invalid severity level")
		}
	}

	for _, timeRange := range c.TimeRanges {
		if err := timeRange.Verify(); err != nil {
			return err
		}
	}

	for _, label := range c.LabelKeys {
		if err := label.Verify(); err != nil {
			return err
		}
	}

	return nil
}

func (t *TimeRanges) Verify() error {
	if t.Start == "" {
		return errors.New("start time cannot be empty")
	}
	if t.End == "" {
		return errors.New("end time cannot be empty")
	}

	// 进一步校验时间格式或检查时间段的合理性

	return nil
}

func (l *LabelFilter) Verify() error {
	if l.Key == "" {
		return errors.New("label key cannot be empty")
	}
	if l.Op == "" {
		return errors.New("operation cannot be empty")
	}
	if l.Op != "==" && l.Op != "!=" && l.Op != "in" && l.Op != "not in" &&
		l.Op != "=~" && l.Op != "!~" {
		return errors.New("invalid operation")
	}
	if l.Value == "" {
		return errors.New("value cannot be empty")
	}
	return nil
}

func (r *NotifyRule) Update(ctx *ctx.Context, ref NotifyRule) error {
	// ref.FE2DB()

	ref.ID = r.ID
	ref.CreateAt = r.CreateAt
	ref.CreateBy = r.CreateBy
	ref.UpdateAt = time.Now().Unix()

	err := ref.Verify()
	if err != nil {
		return err
	}
	return DB(ctx).Model(r).Select("*").Updates(ref).Error
}

func NotifyRuleGet(ctx *ctx.Context, where string, args ...interface{}) (*NotifyRule, error) {
	lst, err := NotifyRulesGet(ctx, where, args...)
	if err != nil || len(lst) == 0 {
		return nil, err
	}
	return lst[0], err
}

func NotifyRulesGet(ctx *ctx.Context, where string, args ...interface{}) ([]*NotifyRule, error) {
	lst := make([]*NotifyRule, 0)
	session := DB(ctx)
	if where != "" && len(args) > 0 {
		session = session.Where(where, args...)
	}
	err := session.Find(&lst).Error
	if err != nil {
		return nil, err
	}
	return lst, nil
}

type NotifyRuleChecker interface {
	IfUsed(*NotifyRule) bool
}

func UsedByNotifyRule(ctx *ctx.Context, nrc NotifyRuleChecker) ([]int64, error) {
	notifyRules, err := NotifyRulesGet(ctx, "", nil)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0)
	for _, nr := range notifyRules {
		if nrc.IfUsed(nr) {
			ids = append(ids, nr.ID)
		}
	}
	return ids, nil
}
