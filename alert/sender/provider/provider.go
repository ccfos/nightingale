package provider

import (
	"context"
	"net/http"

	"github.com/ccfos/nightingale/v6/models"
)

// NotifyChannelProvider 通知媒介提供者接口
// 新增媒介只需实现此接口并注册即可
type NotifyChannelProvider interface {
	// Ident 返回媒介标识，与 notify_channel 表的 ident 字段对应
	// 如 "dingtalk", "wecom", "email", "flashduty", "http"
	Ident() string

	// Check 校验通知媒介配置是否合法 (保存前调用)
	Check(config *models.NotifyChannelConfig) error

	// Notify 发送通知
	Notify(ctx context.Context, req *NotifyRequest) *NotifyResult
}

type NotifyRequest struct {
	NotifyRuleId         int64
	Config               *models.NotifyChannelConfig
	Events               []*models.AlertCurEvent
	TplContent           map[string]interface{}
	FlashDutyChannelIDs  []int64
	PagerDutyRoutingKeys []string
	CustomParams         map[string]string
	Sendtos              []string
	ImGroupIDs           []string                  // 飞书群/钉钉群ID
	ImGroupRobotCodes    map[string]string         // 钉钉群 openConversationId -> robotCode, 由 BuildNotifyContext 预取
	HttpClient           *http.Client              // 由 cache 层提供
	SmtpChan             chan *models.EmailContext // 由 cache 层提供 (仅 smtp 类型)
	SiteUrl              string
}

type NotifyResult struct {
	Target   string // 发送目标 (用于 NotifyRecord)
	Response string // 响应内容
	Err      error
}
