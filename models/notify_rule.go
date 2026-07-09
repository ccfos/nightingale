package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

	PipelineConfigs []PipelineConfig `json:"pipeline_configs" gorm:"serializer:json"`

	// 通知配置
	NotifyConfigs []NotifyConfig `json:"notify_configs" gorm:"serializer:json"`
	ExtraConfig   interface{}    `json:"extra_config,omitempty" gorm:"serializer:json"`

	CreateAt         int64  `json:"create_at"`
	CreateBy         string `json:"create_by"`
	UpdateAt         int64  `json:"update_at"`
	UpdateBy         string `json:"update_by"`
	UpdateByNickname string `json:"update_by_nickname" gorm:"-"`
}

type PipelineConfig struct {
	PipelineId int64 `json:"pipeline_id"`
	Enable     bool  `json:"enable"`
}

func (r *NotifyRule) TableName() string {
	return "notify_rule"
}

type NotifyConfig struct {
	ChannelID  int64                  `json:"channel_id"`  // 通知媒介(如：阿里云短信)
	TemplateID int64                  `json:"template_id"` // 通知模板
	Params     map[string]interface{} `json:"params"`      // 通知参数
	Type       string                 `json:"type"`

	Severities []int        `json:"severities"`  // 适用级别(一级告警、二级告警、三级告警)
	TimeRanges []TimeRanges `json:"time_ranges"` // 适用时段
	LabelKeys  []TagFilter  `json:"label_keys"`  // 适用标签
	Attributes []TagFilter  `json:"attributes"`  // 适用属性

	// 仅用于前端展示，不持久化：读接口按 params 里的 user_ids/user_group_ids
	// 解析出的用户昵称与用户组名（邮件/短信/电话等 user-info 媒介），以及 channel_id
	// 对应的媒介标识 ident。写接口会清空，避免回显数据被序列化进 DB。
	ChannelIdent   string   `json:"channel_ident,omitempty" gorm:"-"`
	UserNames      []string `json:"user_names,omitempty" gorm:"-"`
	UserGroupNames []string `json:"user_group_names,omitempty" gorm:"-"`
}

// CleanFEFields 清除仅用于前端展示的计算字段，写库前调用。
// NotifyConfigs 以 serializer:json 整体入库，gorm:"-" 对其内部字段无效，
// 故需显式清空，防止前端回显的 channel_ident/user_names/user_group_names 被持久化。
func (r *NotifyRule) CleanFEFields() {
	for i := range r.NotifyConfigs {
		r.NotifyConfigs[i].ChannelIdent = ""
		r.NotifyConfigs[i].UserNames = nil
		r.NotifyConfigs[i].UserGroupNames = nil
	}
}

// ParseUserIDs 从 params 中解析 user_ids（邮件/短信/电话等 user-info 媒介）。
func (n *NotifyConfig) ParseUserIDs() []int64 {
	return parseInt64IDs(n.Params["user_ids"])
}

// ParseUserGroupIDs 从 params 中解析 user_group_ids。
func (n *NotifyConfig) ParseUserGroupIDs() []int64 {
	return parseInt64IDs(n.Params["user_group_ids"])
}

func parseInt64IDs(value interface{}) []int64 {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var ids []int64
	if json.Unmarshal(data, &ids) != nil {
		return nil
	}
	return ids
}

func (n *NotifyConfig) Hash() string {
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%d%d%v%s%v%v%v%v", n.ChannelID, n.TemplateID, n.Params, n.Type, n.Severities, n.TimeRanges, n.LabelKeys, n.Attributes)))
	return hex.EncodeToString(hash.Sum(nil))
}

type CustomParams struct {
	UserIDs      []int64 `json:"user_ids"`
	UserGroupIDs []int64 `json:"user_group_ids"`
	IDs          []int64 `json:"ids"`
}

type TimeRanges struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Week  []int  `json:"week"`
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

	// if len(r.UserGroupIds) == 0 {
	// 	return errors.New("user group ids cannot be empty")
	// }

	// if len(r.NotifyConfigs) == 0 {
	// 	return errors.New("notify configs cannot be empty")
	// }

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

	db := DB(ctx).Model(r).Select("*")
	if ref.ExtraConfig == nil {
		db = db.Omit("ExtraConfig")
	}
	return db.Updates(ref).Error
}

func (r *NotifyRule) DB2FE() {
	if r.UserGroupIds == nil {
		r.UserGroupIds = make([]int64, 0)
	}
	if r.NotifyConfigs == nil {
		r.NotifyConfigs = make([]NotifyConfig, 0)
	}
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
	err := session.Order("name asc").Find(&lst).Error
	if err != nil {
		return nil, err
	}
	for _, r := range lst {
		r.DB2FE()
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
