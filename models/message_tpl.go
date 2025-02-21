package models

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

// MessageTemplate 消息模板结构
type MessageTemplate struct {
	ID                 int64             `json:"id" gorm:"primarykey"`
	Name               string            `json:"name"`                           // 模板名称
	Ident              string            `json:"ident"`                          // 模板标识
	Content            map[string]string `json:"content" gorm:"serializer:json"` // 模板内容
	UserGroupIds       []int64           `json:"user_group_ids" gorm:"serializer:json"`
	NotifyChannelIdent string            `json:"notify_channel_ident"` // 通知媒介 Ident
	Private            int               `json:"private"`              // 0-公开 1-私有
	CreateAt           int64             `json:"create_at"`
	CreateBy           string            `json:"create_by"`
	UpdateAt           int64             `json:"update_at"`
	UpdateBy           string            `json:"update_by"`
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

func (t *MessageTemplate) TableName() string {
	return "message_template"
}

func (t *MessageTemplate) Verify() error {
	if t.Name == "" {
		return errors.New("template name cannot be empty")
	}

	if t.Ident == "" {
		return errors.New("template identifier cannot be empty")
	}

	// if len(t.Content) == 0 {
	// 	return errors.New("template content cannot be empty")
	// }

	for key, value := range t.Content {
		if key == "" || value == "" {
			return errors.New("template content cannot have empty keys or values")
		}
	}

	if t.Private == 1 && len(t.UserGroupIds) == 0 {
		return errors.New("user group IDs of private msg tpl cannot be empty")
	}

	if t.Private != 0 && t.Private != 1 {
		return errors.New("private flag must be 0 or 1")
	}

	return nil
}

func (t *MessageTemplate) Update(ctx *ctx.Context, ref MessageTemplate) error {
	// ref.FE2DB()
	if t.Ident != ref.Ident {
		return errors.New("cannot update ident")
	}

	ref.ID = t.ID
	ref.CreateAt = t.CreateAt
	ref.CreateBy = t.CreateBy
	ref.UpdateAt = time.Now().Unix()

	err := ref.Verify()
	if err != nil {
		return err
	}
	return DB(ctx).Model(t).Select("*").Updates(ref).Error
}

func (t *MessageTemplate) DB2FE() {
	if t.UserGroupIds == nil {
		t.UserGroupIds = make([]int64, 0)
	}
}

func MessageTemplateGet(ctx *ctx.Context, where string, args ...interface{}) (*MessageTemplate, error) {
	lst, err := MessageTemplatesGet(ctx, where, args...)
	if err != nil || len(lst) == 0 {
		return nil, err
	}
	return lst[0], err
}

func MessageTemplatesGet(ctx *ctx.Context, where string, args ...interface{}) ([]*MessageTemplate, error) {
	lst := make([]*MessageTemplate, 0)
	session := DB(ctx)
	if where != "" && len(args) > 0 {
		session = session.Where(where, args...)
	}

	err := session.Find(&lst).Error
	if err != nil {
		return nil, err
	}
	for _, t := range lst {
		t.DB2FE()
	}
	return lst, nil
}

func MessageTemplatesGetBy(ctx *ctx.Context, notifyChannelIdents []string) ([]*MessageTemplate, error) {
	lst := make([]*MessageTemplate, 0)
	session := DB(ctx)
	if len(notifyChannelIdents) > 0 {
		session = session.Where("notify_channel_ident IN (?)", notifyChannelIdents)
	}

	err := session.Find(&lst).Error
	if err != nil {
		return nil, err
	}
	for _, t := range lst {
		t.DB2FE()
	}
	return lst, nil
}

type MsgTplList []*MessageTemplate

func (t MsgTplList) GetIdentSet() map[int64]struct{} {
	idents := make(map[int64]struct{}, len(t))
	for _, tpl := range t {
		idents[tpl.ID] = struct{}{}
	}
	return idents
}

func (t MsgTplList) IfUsed(nr *NotifyRule) bool {
	identSet := t.GetIdentSet()
	for _, nc := range nr.NotifyConfigs {
		if _, ok := identSet[nc.TemplateID]; ok {
			return true
		}
	}
	return false
}

const (
	DingtalkTitle   = `{{if .IsRecovered}}Recovered{{else}}Triggered{{end}}: {{.RuleName}} {{.TagsJSON}}`
	FeishuCardTitle = `🔔 {{.RuleName}}`
)

var MsgTplMap = map[string]map[string]string{
	Dingtalk:   {"title": DingtalkTitle, "body": TplMap[Dingtalk]},
	Email:      {"title": TplMap[EmailSubject], "body": TplMap[Email]},
	FeishuCard: {"title": FeishuCardTitle, "body": TplMap[FeishuCard]},
	Feishu:     {"body": TplMap[Feishu]}, Wecom: {"body": TplMap[Wecom]},
}

func InitMessageTemplate(ctx *ctx.Context) {
	if !ctx.IsCenter {
		return
	}

	for channel, content := range MsgTplMap {
		msgTpl := MessageTemplate{
			Name:               channel,
			Ident:              channel,
			Content:            content,
			NotifyChannelIdent: channel,
			CreateBy:           "system",
			CreateAt:           time.Now().Unix(),
			UpdateBy:           "system",
			UpdateAt:           time.Now().Unix(),
		}

		err := msgTpl.Upsert(ctx, channel)
		if err != nil {
			logger.Warningf("failed to upsert msg tpls %v", err)
		}
	}
}

func (t *MessageTemplate) Upsert(ctx *ctx.Context, ident string) error {
	tpl, err := MessageTemplateGet(ctx, "ident = ?", ident)
	if err != nil {
		return errors.WithMessage(err, "failed to get message tpl")
	}
	if tpl == nil {
		return Insert(ctx, t)
	}

	if tpl.UpdateBy != "" && tpl.UpdateBy != "system" {
		return nil
	}
	return tpl.Update(ctx, *t)
}
