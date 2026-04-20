package models

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"gorm.io/gorm/clause"
)

const (
	DingtalkGroupStatusInstalled   = 1
	DingtalkGroupStatusUninstalled = 0
)

// DingtalkGroup 钉钉酷应用场景群安装状态及群信息，按 client_id（AppKey）+ 会话维度唯一。
// todo 不上线，不加到 migrate
type DingtalkGroup struct {
	ID                     int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	ClientID               string `json:"client_id" gorm:"type:varchar(128);not null;uniqueIndex:uk_dt_group_client_conv,priority:1;comment:钉钉应用 ClientId(AppKey)"`
	OpenConversationCorpID string `json:"open_conversation_corp_id" gorm:"type:varchar(128);not null;default:'';uniqueIndex:uk_dt_group_client_conv,priority:2"`
	OpenConversationID     string `json:"open_conversation_id" gorm:"type:varchar(128);not null;uniqueIndex:uk_dt_group_client_conv,priority:3"`
	CoolAppCode            string `json:"cool_app_code" gorm:"type:varchar(128);not null;default:''"`
	RobotCode              string `json:"robot_code" gorm:"type:varchar(128);not null;default:''"`
	Title                  string `json:"title" gorm:"type:varchar(255);not null;default:''"`
	Status                 int    `json:"status" gorm:"type:int;not null;default:1;comment:1 installed 0 uninstalled"`
	CreatedAt              int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt              int64  `json:"updated_at" gorm:"type:bigint;not null"`
	UninstalledAt          int64  `json:"uninstalled_at" gorm:"type:bigint;not null;default:0"`
}

func (DingtalkGroup) TableName() string {
	return "dingtalk_group"
}

// UpsertDingtalkGroupInstall 安装事件：写入/更新群信息。
func UpsertDingtalkGroupInstall(c *ctx.Context, row *DingtalkGroup) error {
	if c == nil || c.DB == nil || row == nil {
		return nil
	}
	now := time.Now().Unix()
	if row.CreatedAt == 0 {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	row.Status = DingtalkGroupStatusInstalled
	row.UninstalledAt = 0

	return DB(c).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "client_id"},
			{Name: "open_conversation_corp_id"},
			{Name: "open_conversation_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"cool_app_code", "robot_code",
			"title",
			"status", "updated_at", "uninstalled_at",
		}),
	}).Create(row).Error
}

// MarkDingtalkGroupUninstall 卸载事件：标记为已卸载。
func MarkDingtalkGroupUninstall(c *ctx.Context, clientID, openConversationCorpID, openConversationID string) error {
	if c == nil || c.DB == nil || clientID == "" || openConversationID == "" {
		return nil
	}
	now := time.Now().Unix()
	res := DB(c).Model(&DingtalkGroup{}).
		Where("client_id = ? AND open_conversation_corp_id = ? AND open_conversation_id = ?", clientID, openConversationCorpID, openConversationID).
		Updates(map[string]interface{}{
			"status":         DingtalkGroupStatusUninstalled,
			"uninstalled_at": now,
			"updated_at":     now,
		})
	if res.Error != nil {
		return res.Error
	}
	return nil
}

// DingtalkGroupsGetByClientID 按 client_id 查询群列表；onlyInstalled 为 true 时仅返回已安装。
func DingtalkGroupsGetByClientID(c *ctx.Context, clientID string, onlyInstalled bool) ([]*DingtalkGroup, error) {
	lst := make([]*DingtalkGroup, 0)
	if c == nil || c.DB == nil || clientID == "" {
		return lst, nil
	}
	session := DB(c).Where("client_id = ?", clientID)
	if onlyInstalled {
		session = session.Where("status = ?", DingtalkGroupStatusInstalled)
	}
	err := session.Order("title asc").Order("open_conversation_id asc").Find(&lst).Error
	return lst, err
}

// DingtalkGroupRobotCodes 批量查询 (client_id, open_conversation_id) 对应的 robot_code。
// 返回 map[openConversationID]robotCode，只包含 status=installed 且 robot_code 非空的记录。
// 钉钉 robotCode 是在酷应用被装进群时由 Stream 事件推送过来的，按群存，
// 不等同于 AppKey —— 发群聊消息时必须用这份映射而不是用 AppKey 兜底。
func DingtalkGroupRobotCodes(c *ctx.Context, clientID string, openConversationIDs []string) (map[string]string, error) {
	result := make(map[string]string)
	if c == nil || c.DB == nil || clientID == "" || len(openConversationIDs) == 0 {
		return result, nil
	}
	lst := make([]*DingtalkGroup, 0, len(openConversationIDs))
	err := DB(c).
		Where("client_id = ? AND status = ?", clientID, DingtalkGroupStatusInstalled).
		Where("open_conversation_id IN ?", openConversationIDs).
		Find(&lst).Error
	if err != nil {
		return nil, err
	}
	for _, g := range lst {
		if g.RobotCode != "" {
			result[g.OpenConversationID] = g.RobotCode
		}
	}
	return result, nil
}

// DingtalkGroupsGetByClientIDPage 按 client_id 分页查询；onlyInstalled 语义同 DingtalkGroupsGetByClientID。
func DingtalkGroupsGetByClientIDPage(c *ctx.Context, clientID string, onlyInstalled bool, offset, limit int) ([]*DingtalkGroup, int64, error) {
	lst := make([]*DingtalkGroup, 0)
	if c == nil || c.DB == nil || clientID == "" {
		return lst, 0, nil
	}
	session := DB(c).Where("client_id = ?", clientID)
	if onlyInstalled {
		session = session.Where("status = ?", DingtalkGroupStatusInstalled)
	}

	var total int64
	if err := session.Model(&DingtalkGroup{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := session.Order("title asc").Order("open_conversation_id asc").Offset(offset).Limit(limit).Find(&lst).Error
	return lst, total, err
}
