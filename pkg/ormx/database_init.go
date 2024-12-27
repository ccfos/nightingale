package ormx

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

type InitUser struct {
	ID             uint64         `gorm:"primaryKey;autoIncrement"`
	Username       string         `gorm:"size:64;not null;unique;comment:login name, cannot rename;uniqueIndex"`
	Nickname       string         `gorm:"size:64;not null;comment:display name, chinese name"`
	Password       string         `gorm:"size:128;not null;default:''"`
	Phone          string         `gorm:"size:16;not null;default:''"`
	Email          string         `gorm:"size:64;not null;default:''"`
	Portrait       string         `gorm:"size:255;not null;default:'';comment:portrait image url"`
	Roles          string         `gorm:"size:255;not null;comment:Admin | Standard | Guest, split by space"`
	Contacts       sql.NullString `gorm:"size:1024;default null;comment:json e.g. {wecom:xx, dingtalk_robot_token:yy}"`
	Maintainer     bool           `gorm:"type:tinyint(1);not null;default:0"`
	Belong         string         `gorm:"size:16;not null;default:'';comment:belong"`
	LastActiveTime int64          `gorm:"not null;default:0"`
	CreateAt       int64          `gorm:"not null;default:0"`
	CreateBy       string         `gorm:"size:64;not null;default:''"`
	UpdateAt       int64          `gorm:"not null;default:0"`
	UpdateBy       string         `gorm:"size:64;not null;default:''"`
}

func (InitUser) TableName() string {
	return "users"
}

func (InitUser) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresUser struct {
	ID             uint64         `gorm:"primaryKey;autoIncrement"`
	Username       string         `gorm:"size:64;not null;unique;comment:login name, cannot rename;uniqueIndex"`
	Nickname       string         `gorm:"size:64;not null;comment:display name, chinese name"`
	Password       string         `gorm:"size:128;not null;default:''"`
	Phone          string         `gorm:"size:16;not null;default:''"`
	Email          string         `gorm:"size:64;not null;default:''"`
	Portrait       string         `gorm:"size:255;not null;default:'';comment:portrait image url"`
	Roles          string         `gorm:"size:255;not null;comment:Admin | Standard | Guest, split by space"`
	Contacts       sql.NullString `gorm:"size:1024;default null;comment:json e.g. {wecom:xx, dingtalk_robot_token:yy}"`
	Maintainer     int16          `gorm:"type:smallint;not null;default:0"`
	Belong         string         `gorm:"size:16;not null;default:'';comment:belong"`
	LastActiveTime int64          `gorm:"not null;default:0"`
	CreateAt       int64          `gorm:"not null;default:0"`
	CreateBy       string         `gorm:"size:64;not null;default:''"`
	UpdateAt       int64          `gorm:"not null;default:0"`
	UpdateBy       string         `gorm:"size:64;not null;default:''"`
}

func (InitPostgresUser) TableName() string {
	return "users"
}

type InitUserGroup struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	Name     string `gorm:"size:128;not null;default:''"`
	Note     string `gorm:"size:255;not null;default:''"`
	CreateAt int64  `gorm:"not null;default:0;index"`
	CreateBy string `gorm:"size:64;not null;default:''"`
	UpdateAt int64  `gorm:"not null;default:0;index"`
	UpdateBy string `gorm:"size:64;not null;default:''"`
}

func (InitUserGroup) TableName() string {
	return "user_group"
}

func (InitUserGroup) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitUserGroupMember struct {
	ID      uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID int64  `gorm:"not null;index"`
	UserID  uint64 `gorm:"not null;index"`
}

func (InitUserGroupMember) TableName() string {
	return "user_group_member"
}

func (InitUserGroupMember) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitConfig struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	CKey      string `gorm:"column:ckey;size:191;not null"`
	CVal      string `gorm:"column:cval;type:text;not null"`
	Note      string `gorm:"size:1024;not null;default:''"`
	External  bool   `gorm:"type:tinyint(1);not null;default:0"`
	Encrypted bool   `gorm:"type:tinyint(1);not null;default:0"`
	CreateAt  int64  `gorm:"not null;default:0"`
	CreateBy  string `gorm:"size:64;not null;default:''"`
	UpdateAt  int64  `gorm:"not null;default:0"`
	UpdateBy  string `gorm:"size:64;not null;default:''"`
}

func (InitConfig) TableName() string {
	return "configs"
}

func (InitConfig) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresConfig struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	CKey      string `gorm:"column:ckey;size:191;not null"`
	CVal      string `gorm:"column:cval;type:text;not null"`
	Note      string `gorm:"size:1024;not null;default:''"`
	External  int16  `gorm:"type:smallint;not null;default:0"`
	Encrypted int16  `gorm:"type:smallint;not null;default:0"`
	CreateAt  int64  `gorm:"not null;default:0"`
	CreateBy  string `gorm:"size:64;not null;default:''"`
	UpdateAt  int64  `gorm:"not null;default:0"`
	UpdateBy  string `gorm:"size:64;not null;default:''"`
}

func (InitPostgresConfig) TableName() string {
	return "configs"
}

type InitRole struct {
	ID   uint64 `gorm:"primaryKey;autoIncrement"`
	Name string `gorm:"size:191;not null;default:'';uniqueIdx"`
	Note string `gorm:"size:255;not null;default:''"`
}

func (InitRole) TableName() string {
	return "role"
}

func (InitRole) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitRoleOperation struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	RoleName  string `gorm:"size:128;not null;index"`
	Operation string `gorm:"size:191;not null;index"`
}

func (InitRoleOperation) TableName() string {
	return "role_operation"
}

func (InitRoleOperation) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitBusiGroup struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	Name        string `gorm:"size:191;not null;uniqueIndex"`
	LabelEnable bool   `gorm:"type:tinyint(1);not null;default:0"`
	LabelValue  string `gorm:"size:191;not null;default:'';comment:if label_enable: label_value can not be blank"`
	CreateAt    int64  `gorm:"not null;default:0"`
	CreateBy    string `gorm:"size:64;not null;default:''"`
	UpdateAt    int64  `gorm:"not null;default:0"`
	UpdateBy    string `gorm:"size:64;not null;default:''"`
}

func (InitBusiGroup) TableName() string {
	return "busi_group"
}

func (InitBusiGroup) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresBusiGroup struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	Name        string `gorm:"size:191;not null;uniqueIndex"`
	LabelEnable int16  `gorm:"type:smallint;not null;default:0"`
	LabelValue  string `gorm:"size:191;not null;default:'';comment:if label_enable: label_value can not be blank"`
	CreateAt    int64  `gorm:"not null;default:0"`
	CreateBy    string `gorm:"size:64;not null;default:''"`
	UpdateAt    int64  `gorm:"not null;default:0"`
	UpdateBy    string `gorm:"size:64;not null;default:''"`
}

func (InitPostgresBusiGroup) TableName() string {
	return "busi_group"
}

type InitBusiGroupMember struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	BusiGroupID int64  `gorm:"not null;comment:busi group id;index"`
	UserGroupID int64  `gorm:"not null;comment:user group id;index"`
	PermFlag    string `gorm:"size:2;not null;comment:ro | rw"`
}

func (InitBusiGroupMember) TableName() string {
	return "busi_group_member"
}

func (InitBusiGroupMember) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitBoard struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID  uint64 `gorm:"not null;default:0;comment:busi group id;uniqueIndex:idx_groupid_name"`
	Name     string `gorm:"size:191;not null;uniqueIndex:idx_groupid_name"`
	Ident    string `gorm:"size:200;not null;default:'';index"`
	Tags     string `gorm:"size:255;not null;comment:split by space"`
	Public   bool   `gorm:"type:tinyint(1);not null;default:0;comment:0:false 1:true"`
	BuiltIn  bool   `gorm:"type:tinyint(1);not null;default:0;comment:0:false 1:true"`
	Hide     bool   `gorm:"type:tinyint(1);not null;default:0;comment:0:false 1:true"`
	CreateAt int64  `gorm:"not null;default:0"`
	CreateBy string `gorm:"size:64;not null;default:''"`
	UpdateAt int64  `gorm:"not null;default:0"`
	UpdateBy string `gorm:"size:64;not null;default:''"`
}

func (InitBoard) TableName() string {
	return "board"
}

func (InitBoard) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresBoard struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID  uint64 `gorm:"not null;default:0;comment:busi group id;uniqueIndex:idx_groupid_name"`
	Name     string `gorm:"size:191;not null;uniqueIndex:idx_groupid_name"`
	Ident    string `gorm:"size:200;not null;default:'';index"`
	Tags     string `gorm:"size:255;not null;comment:split by space"`
	Public   int16  `gorm:"type:smallint;not null;default:0;comment:0:false 1:true"`
	BuiltIn  int16  `gorm:"type:smallint;not null;default:0;comment:0:false 1:true"`
	Hide     int16  `gorm:"type:smallint;not null;default:0;comment:0:false 1:true"`
	CreateAt int64  `gorm:"not null;default:0"`
	CreateBy string `gorm:"size:64;not null;default:''"`
	UpdateAt int64  `gorm:"not null;default:0"`
	UpdateBy string `gorm:"size:64;not null;default:''"`
}

func (InitPostgresBoard) TableName() string {
	return "board"
}

type InitBoardPayload struct {
	ID      uint64 `gorm:"not null;comment:dashboard id"`
	Payload string `gorm:"type:mediumtext;not null"`
}

func (InitBoardPayload) TableName() string {
	return "board_payload"
}

func (InitBoardPayload) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresBoardPayload struct {
	ID      uint64 `gorm:"primaryKey;comment:dashboard id"`
	Payload string `gorm:"type:TEXT;not null"`
}

func (InitPostgresBoardPayload) TableName() string {
	return "board_payload"
}

type InitDashboard struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID  uint64 `gorm:"not null;default:0;comment:busi group id;uniqueIndex:idx_group_name"`
	Name     string `gorm:"size:191;not null;uniqueIndex:idx_group_name"`
	Tags     string `gorm:"size:255;not null;comment:split by space"`
	Configs  string `gorm:"size:8192;comment:dashboard variables"`
	CreateAt int64  `gorm:"not null;default:0"`
	CreateBy string `gorm:"size:64;not null;default:''"`
	UpdateAt int64  `gorm:"not null;default:0"`
	UpdateBy string `gorm:"size:64;not null;default:''"`
}

func (InitDashboard) TableName() string {
	return "dashboard"
}

func (InitDashboard) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitChartGroup struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	DashboardID uint64 `gorm:"not null;index"`
	Name        string `gorm:"size:255;not null"`
	Weight      int32  `gorm:"not null;default:0"`
}

func (InitChartGroup) TableName() string {
	return "chart_group"
}

func (InitChartGroup) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitChart struct {
	ID      uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID int64  `gorm:"not null;comment:chart group id;index"`
	Configs string `gorm:"type:text"`
	Weight  int32  `gorm:"not null;default:0"`
}

func (InitChart) TableName() string {
	return "chart"
}

func (InitChart) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitChartShare struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	Cluster      string `gorm:"size:128;not null"`
	DatasourceID int64  `gorm:"not null;default:0"`
	Configs      string `gorm:"type:text"`
	CreateAt     int64  `gorm:"not null;default:0;index"`
	CreateBy     string `gorm:"size:64;not null;default:''"`
}

func (InitChartShare) TableName() string {
	return "chart_share"
}

func (InitChartShare) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitAlertRule struct {
	ID                uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID           uint64 `gorm:"not null;default:0;comment:busi group id;index"`
	Cate              string `gorm:"size:128;not null"`
	DatasourceIDs     string `gorm:"size:255;not null;default:'';comment:datasource ids"`
	Cluster           string `gorm:"size:128;not null"`
	Name              string `gorm:"size:255;not null"`
	Note              string `gorm:"size:1024;not null;default:''"`
	Prod              string `gorm:"size:255;not null;default:''"`
	Algorithm         string `gorm:"size:255;not null;default:''"`
	AlgoParams        string `gorm:"size:255"`
	Delay             int32  `gorm:"not null;default:0"`
	Severity          int16  `gorm:"type:tinyint(1);not null;comment:1:Emergency 2:Warning 3:Notice"`
	Disabled          bool   `gorm:"type:tinyint(1);not null;comment:0:enabled 1:disabled"`
	PromForDuration   int32  `gorm:"not null;comment:prometheus for, unit:s"`
	RuleConfig        string `gorm:"type:text;not null;comment:rule_config"`
	PromQL            string `gorm:"type:text;not null;comment:promql"`
	PromEvalInterval  int32  `gorm:"not null;comment:evaluate interval"`
	EnableStime       string `gorm:"size:255;not null;default:'00:00'"`
	EnableEtime       string `gorm:"size:255;not null;default:'23:59'"`
	EnableDaysOfWeek  string `gorm:"size:255;not null;default:'';comment:split by space: 0 1 2 3 4 5 6"`
	EnableInBg        bool   `gorm:"type:tinyint(1);not null;default:0;comment:1: only this bg 0: global"`
	NotifyRecovered   bool   `gorm:"type:tinyint(1);not null;comment:whether notify when recovery"`
	NotifyChannels    string `gorm:"size:255;not null;default:'';comment:split by space: sms voice email dingtalk wecom"`
	NotifyGroups      string `gorm:"size:255;not null;default:'';comment:split by space: 233 43"`
	NotifyRepeatStep  int32  `gorm:"not null;default:0;comment:unit: min"`
	NotifyMaxNumber   int32  `gorm:"not null;default:0"`
	RecoverDuration   int32  `gorm:"not null;default:0;comment:unit: s"`
	Callbacks         string `gorm:"size:4096;not null;default:'';comment:split by space: http://a.com/api/x http://a.com/api/y"`
	RunbookURL        string `gorm:"size:4096"`
	AppendTags        string `gorm:"size:255;not null;default:'';comment:split by space: service=n9e mod=api"`
	Annotations       string `gorm:"type:text;not null;comment:annotations"`
	ExtraConfig       string `gorm:"type:text;not null;comment:extra_config"`
	CreateAt          int64  `gorm:"not null;default:0"`
	CreateBy          string `gorm:"size:64;not null;default:''"`
	UpdateAt          int64  `gorm:"not null;default:0;index"`
	UpdateBy          string `gorm:"size:64;not null;default:''"`
	DatasourceQueries string `gorm:"type:text"`
}

func (InitAlertRule) TableName() string {
	return "alert_rule"
}

func (InitAlertRule) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresAlertRule struct {
	ID                uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID           uint64 `gorm:"not null;default:0;comment:busi group id;index"`
	Cate              string `gorm:"size:128;not null"`
	DatasourceIDs     string `gorm:"size:255;not null;default:'';comment:datasource ids"`
	Cluster           string `gorm:"size:128;not null"`
	Name              string `gorm:"size:255;not null"`
	Note              string `gorm:"size:1024;not null;default:''"`
	Prod              string `gorm:"size:255;not null;default:''"`
	Algorithm         string `gorm:"size:255;not null;default:''"`
	AlgoParams        string `gorm:"size:255"`
	Delay             int32  `gorm:"not null;default:0"`
	Severity          int16  `gorm:"type:smallint;not null;comment:1:Emergency 2:Warning 3:Notice"`
	Disabled          int16  `gorm:"type:smallint;not null;comment:0:enabled 1:disabled"`
	PromForDuration   int32  `gorm:"not null;comment:prometheus for, unit:s"`
	RuleConfig        string `gorm:"type:text;not null;comment:rule_config"`
	PromQL            string `gorm:"type:text;not null;comment:promql"`
	PromEvalInterval  int32  `gorm:"not null;comment:evaluate interval"`
	EnableStime       string `gorm:"size:255;not null;default:'00:00'"`
	EnableEtime       string `gorm:"size:255;not null;default:'23:59'"`
	EnableDaysOfWeek  string `gorm:"size:255;not null;default:'';comment:split by space: 0 1 2 3 4 5 6"`
	EnableInBg        int16  `gorm:"type:smallint;not null;default:0;comment:1: only this bg 0: global"`
	NotifyRecovered   int16  `gorm:"type:smallint;not null;comment:whether notify when recovery"`
	NotifyChannels    string `gorm:"size:255;not null;default:'';comment:split by space: sms voice email dingtalk wecom"`
	NotifyGroups      string `gorm:"size:255;not null;default:'';comment:split by space: 233 43"`
	NotifyRepeatStep  int32  `gorm:"not null;default:0;comment:unit: min"`
	NotifyMaxNumber   int32  `gorm:"not null;default:0"`
	RecoverDuration   int32  `gorm:"not null;default:0;comment:unit: s"`
	Callbacks         string `gorm:"size:4096;not null;default:'';comment:split by space: http://a.com/api/x http://a.com/api/y"`
	RunbookURL        string `gorm:"size:4096"`
	AppendTags        string `gorm:"size:255;not null;default:'';comment:split by space: service=n9e mod=api"`
	Annotations       string `gorm:"type:text;not null;comment:annotations"`
	ExtraConfig       string `gorm:"type:text;not null;comment:extra_config"`
	CreateAt          int64  `gorm:"not null;default:0"`
	CreateBy          string `gorm:"size:64;not null;default:''"`
	UpdateAt          int64  `gorm:"not null;default:0;index"`
	UpdateBy          string `gorm:"size:64;not null;default:''"`
	DatasourceQueries string `gorm:"type:text"`
}

func (InitPostgresAlertRule) TableName() string {
	return "alert_rule"
}

type InitAlertMute struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID       uint64 `gorm:"not null;default:0;comment:busi group id;index"`
	Prod          string `gorm:"size:255;not null;default:''"`
	Note          string `gorm:"size:1024;not null;default:''"`
	Cate          string `gorm:"size:128;not null"`
	Cluster       string `gorm:"size:128;not null"`
	DatasourceIDs string `gorm:"size:255;not null;default:'';comment:datasource ids"`
	Tags          string `gorm:"size:4096;default:'[]';comment:json,map,tagkey->regexp|value"`
	Cause         string `gorm:"size:255;not null;default:''"`
	BTime         int64  `gorm:"column:btime;not null;default:0;comment:begin time"`
	ETime         int64  `gorm:"column:etime;not null;default:0;comment:end time"`
	Disabled      bool   `gorm:"type:tinyint(1);not null;default:0;comment:0:enabled 1:disabled"`
	MuteTimeType  bool   `gorm:"type:tinyint(1);not null;default:0"`
	PeriodicMutes string `gorm:"size:4096;not null;default:''"`
	Severities    string `gorm:"size:32;not null;default:''"`
	CreateAt      int64  `gorm:"not null;default:0;index"`
	CreateBy      string `gorm:"size:64;not null;default:''"`
	UpdateAt      int64  `gorm:"not null;default:0"`
	UpdateBy      string `gorm:"size:64;not null;default:''"`
}

func (InitAlertMute) TableName() string {
	return "alert_mute"
}

func (InitAlertMute) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresAlertMute struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID       uint64 `gorm:"not null;default:0;comment:busi group id;index"`
	Prod          string `gorm:"size:255;not null;default:''"`
	Note          string `gorm:"size:1024;not null;default:''"`
	Cate          string `gorm:"size:128;not null"`
	Cluster       string `gorm:"size:128;not null"`
	DatasourceIDs string `gorm:"size:255;not null;default:'';comment:datasource ids"`
	Tags          string `gorm:"size:4096;default:'[]';comment:json,map,tagkey->regexp|value"`
	Cause         string `gorm:"size:255;not null;default:''"`
	BTime         int64  `gorm:"column:btime;not null;default:0;comment:begin time"`
	ETime         int64  `gorm:"column:etime;not null;default:0;comment:end time"`
	Disabled      int16  `gorm:"type:smallint;not null;default:0;comment:0:enabled 1:disabled"`
	MuteTimeType  int16  `gorm:"type:smallint;not null;default:0"`
	PeriodicMutes string `gorm:"size:4096;not null;default:''"`
	Severities    string `gorm:"size:32;not null;default:''"`
	CreateAt      int64  `gorm:"not null;default:0;index"`
	CreateBy      string `gorm:"size:64;not null;default:''"`
	UpdateAt      int64  `gorm:"not null;default:0"`
	UpdateBy      string `gorm:"size:64;not null;default:''"`
}

func (InitPostgresAlertMute) TableName() string {
	return "alert_mute"
}

type InitAlertSubscribe struct {
	ID               uint64 `gorm:"primaryKey;autoIncrement"`
	Name             string `gorm:"size:255;not null;default:''"`
	Disabled         bool   `gorm:"type:tinyint(1);not null;default:0;comment:0:enabled 1:disabled"`
	GroupID          uint64 `gorm:"not null;default:0;comment:busi group id;index"`
	Prod             string `gorm:"size:255;not null;default:''"`
	Cate             string `gorm:"size:128;not null"`
	DatasourceIDs    string `gorm:"size:255;not null;default:'';comment:datasource ids"`
	Cluster          string `gorm:"size:128;not null"`
	RuleID           int64  `gorm:"not null;default:0"`
	Severities       string `gorm:"size:32;not null;default:''"`
	Tags             string `gorm:"size:4096;not null;default:'';comment:json,map,tagkey->regexp|value"`
	RedefineSeverity int16  `gorm:"type:tinyint(1);default:0;comment:is redefine severity?"`
	NewSeverity      int16  `gorm:"type:tinyint(1);not null;comment:0:Emergency 1:Warning 2:Notice"`
	RedefineChannels int16  `gorm:"type:tinyint(1);default:0;comment:is redefine channels?"`
	NewChannels      string `gorm:"size:255;not null;default:'';comment:split by space: sms voice email dingtalk wecom"`
	UserGroupIDs     string `gorm:"size:250;not null;comment:split by space 1 34 5, notify cc to user_group_ids"`
	BusiGroups       string `gorm:"size:4096;not null;default:'[]'"`
	Note             string `gorm:"size:1024;default:'';comment:note"`
	RuleIDs          string `gorm:"size:1024;default:'';comment:rule_ids"`
	Webhooks         string `gorm:"type:text;not null"`
	ExtraConfig      string `gorm:"type:text;not null;comment:extra_config"`
	RedefineWebhooks bool   `gorm:"type:tinyint(1);default:0"`
	ForDuration      int64  `gorm:"not null;default:0"`
	CreateAt         int64  `gorm:"not null;default:0"`
	CreateBy         string `gorm:"size:64;not null;default:''"`
	UpdateAt         int64  `gorm:"not null;default:0;index"`
	UpdateBy         string `gorm:"size:64;not null;default:''"`
}

func (InitAlertSubscribe) TableName() string {
	return "alert_subscribe"
}

func (InitAlertSubscribe) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresAlertSubscribe struct {
	ID               uint64 `gorm:"primaryKey;autoIncrement"`
	Name             string `gorm:"size:255;not null;default:''"`
	Disabled         int16  `gorm:"type:smallint;not null;default:0;comment:0:enabled 1:disabled"`
	GroupID          uint64 `gorm:"not null;default:0;comment:busi group id;index"`
	Prod             string `gorm:"size:255;not null;default:''"`
	Cate             string `gorm:"size:128;not null"`
	DatasourceIDs    string `gorm:"size:255;not null;default:'';comment:datasource ids"`
	Cluster          string `gorm:"size:128;not null"`
	RuleID           int64  `gorm:"not null;default:0"`
	Severities       string `gorm:"size:32;not null;default:''"`
	Tags             string `gorm:"size:4096;not null;default:'';comment:json,map,tagkey->regexp|value"`
	RedefineSeverity int16  `gorm:"type:smallint;default:0;comment:is redefine severity?"`
	NewSeverity      int16  `gorm:"type:smallint;not null;comment:0:Emergency 1:Warning 2:Notice"`
	RedefineChannels int16  `gorm:"type:smallint;default:0;comment:is redefine channels?"`
	NewChannels      string `gorm:"size:255;not null;default:'';comment:split by space: sms voice email dingtalk wecom"`
	UserGroupIDs     string `gorm:"size:250;not null;comment:split by space 1 34 5, notify cc to user_group_ids"`
	BusiGroups       string `gorm:"size:4096;not null;default:'[]'"`
	Note             string `gorm:"size:1024;default:'';comment:note"`
	RuleIDs          string `gorm:"size:1024;default:'';comment:rule_ids"`
	Webhooks         string `gorm:"type:text;not null"`
	ExtraConfig      string `gorm:"type:text;not null;comment:extra_config"`
	RedefineWebhooks int16  `gorm:"type:smallint;default:0"`
	ForDuration      int64  `gorm:"not null;default:0"`
	CreateAt         int64  `gorm:"not null;default:0"`
	CreateBy         string `gorm:"size:64;not null;default:''"`
	UpdateAt         int64  `gorm:"not null;default:0;index"`
	UpdateBy         string `gorm:"size:64;not null;default:''"`
}

func (InitPostgresAlertSubscribe) TableName() string {
	return "alert_subscribe"
}

type InitTarget struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID      uint64 `gorm:"not null;default:0;comment:busi group id;index"`
	Ident        string `gorm:"size:191;not null;comment:target id;uniqueIndex"`
	Note         string `gorm:"size:255;not null;default:'';comment:append to alert event as field"`
	Tags         string `gorm:"size:512;not null;default:'';comment:append to series data as tags, split by space, append external space at suffix"`
	HostTags     string `gorm:"size:512;not null;default:'';comment:append to series data as tags, split by space, append external space at suffix"`
	HostIP       string `gorm:"size:15;default:'';comment:IPv4 string"`
	AgentVersion string `gorm:"size:255;default:'';comment:agent version"`
	EngineName   string `gorm:"size:255;default:'';comment:engine_name"`
	OS           string `gorm:"size:31;default:'';comment:os type"`
	UpdateAt     int64  `gorm:"not null;default:0"`
}

func (InitTarget) TableName() string {
	return "target"
}

func (InitTarget) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitMetricView struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	Name     string `gorm:"size:191;not null;default:''"`
	Cate     bool   `gorm:"type:tinyint(1);not null;comment:0: preset 1: custom"`
	Configs  string `gorm:"size:8192;not null;default:''"`
	CreateAt int64  `gorm:"not null;default:0"`
	CreateBy uint64 `gorm:"not null;default:0;comment:user id;index"`
	UpdateAt int64  `gorm:"not null;default:0"`
}

func (InitMetricView) TableName() string {
	return "metric_view"
}

func (InitMetricView) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresMetricView struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	Name     string `gorm:"size:191;not null;default:''"`
	Cate     int16  `gorm:"type:smallint;not null;comment:0: preset 1: custom"`
	Configs  string `gorm:"size:8192;not null;default:''"`
	CreateAt int64  `gorm:"not null;default:0"`
	CreateBy uint64 `gorm:"not null;default:0;comment:user id;index"`
	UpdateAt int64  `gorm:"not null;default:0"`
}

func (InitPostgresMetricView) TableName() string {
	return "metric_view"
}

type InitRecordingRule struct {
	ID                uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID           uint64 `gorm:"not null;default:0;comment:group_id;index"`
	DatasourceIDs     string `gorm:"size:255;not null;default:'';comment:datasource ids"`
	Cluster           string `gorm:"size:128;not null"`
	Name              string `gorm:"size:255;not null;comment:new metric name"`
	Note              string `gorm:"size:255;not null;comment:rule note"`
	Disabled          bool   `gorm:"type:tinyint(1);not null;default:0;comment:0:enabled 1:disabled"`
	PromQL            string `gorm:"size:8192;not null;comment:promql"`
	PromEvalInterval  int32  `gorm:"not null;comment:evaluate interval"`
	CronPattern       string `gorm:"size:255;default:'';comment:cron pattern"`
	AppendTags        string `gorm:"size:255;default:'';comment:split by space: service=n9e mod=api"`
	QueryConfigs      string `gorm:"type:text;not null;comment:query configs"`
	CreateAt          int64  `gorm:"default:0"`
	CreateBy          string `gorm:"size:64;default:''"`
	UpdateAt          int64  `gorm:"default:0;index"`
	UpdateBy          string `gorm:"size:64;default:''"`
	DatasourceQueries string `gorm:"type:text"`
}

func (InitRecordingRule) TableName() string {
	return "recording_rule"
}

func (InitRecordingRule) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresRecordingRule struct {
	ID                uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID           uint64 `gorm:"not null;default:0;comment:group_id;index"`
	DatasourceIDs     string `gorm:"size:255;not null;default:'';comment:datasource ids"`
	Cluster           string `gorm:"size:128;not null"`
	Name              string `gorm:"size:255;not null;comment:new metric name"`
	Note              string `gorm:"size:255;not null;comment:rule note"`
	Disabled          int16  `gorm:"type:smallint;not null;default:0;comment:0:enabled 1:disabled"`
	PromQL            string `gorm:"size:8192;not null;comment:promql"`
	PromEvalInterval  int32  `gorm:"not null;comment:evaluate interval"`
	CronPattern       string `gorm:"size:255;default:'';comment:cron pattern"`
	AppendTags        string `gorm:"size:255;default:'';comment:split by space: service=n9e mod=api"`
	QueryConfigs      string `gorm:"type:text;not null;comment:query configs"`
	CreateAt          int64  `gorm:"default:0"`
	CreateBy          string `gorm:"size:64;default:''"`
	UpdateAt          int64  `gorm:"default:0;index"`
	UpdateBy          string `gorm:"size:64;default:''"`
	DatasourceQueries string `gorm:"type:text"`
}

func (InitPostgresRecordingRule) TableName() string {
	return "recording_rule"
}

type InitAlertAggrView struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	Name     string `gorm:"size:191;not null;default:''"`
	Rule     string `gorm:"size:2048;not null;default:''"`
	Cate     bool   `gorm:"type:tinyint(1);not null;comment:0: preset 1: custom"`
	CreateAt int64  `gorm:"not null;default:0"`
	CreateBy int64  `gorm:"not null;default:0;comment:user id;index:create_by"`
	UpdateAt int64  `gorm:"not null;default:0"`
}

func (InitAlertAggrView) TableName() string {
	return "alert_aggr_view"
}

func (InitAlertAggrView) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresAlertAggrView struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	Name     string `gorm:"size:191;not null;default:''"`
	Rule     string `gorm:"size:2048;not null;default:''"`
	Cate     int16  `gorm:"type:smallint;not null;comment:0: preset 1: custom"`
	CreateAt int64  `gorm:"not null;default:0"`
	CreateBy int64  `gorm:"not null;default:0;comment:user id;index:create_by"`
	UpdateAt int64  `gorm:"not null;default:0"`
}

func (InitPostgresAlertAggrView) TableName() string {
	return "alert_aggr_view"
}

type InitAlertCurEvent struct {
	ID               uint64 `gorm:"primaryKey;NOT NULL;COMMENT:use alert_his_event.id"`
	Cate             string `gorm:"size:128;not null"`
	DatasourceID     int64  `gorm:"not null;default:0;comment:datasource id"`
	Cluster          string `gorm:"size:128;not null"`
	GroupID          uint64 `gorm:"not null;comment:busi group id of rule;index"`
	GroupName        string `gorm:"size:255;not null;default:'';comment:busi group name"`
	Hash             string `gorm:"size:64;not null;comment:rule_id + vector_pk;index"`
	RuleID           uint64 `gorm:"not null;index"`
	RuleName         string `gorm:"size:255;not null"`
	RuleNote         string `gorm:"size:2048;not null;default:'alert rule note'"`
	RuleProd         string `gorm:"size:255;not null;default:''"`
	RuleAlgo         string `gorm:"size:255;not null;default:''"`
	Severity         int16  `gorm:"type:tinyint(1);not null;comment:0:Emergency 1:Warning 2:Notice"`
	PromForDuration  int32  `gorm:"not null;comment:prometheus for, unit:s"`
	PromQL           string `gorm:"size:8192;not null;comment:promql"`
	PromEvalInterval int32  `gorm:"not null;comment:evaluate interval"`
	Callbacks        string `gorm:"size:2048;not null;default:'';comment:split by space: http://a.com/api/x http://a.com/api/y"`
	RunbookURL       string `gorm:"size:255"`
	NotifyRecovered  bool   `gorm:"type:tinyint(1);not null;comment:whether notify when recovery"`
	NotifyChannels   string `gorm:"size:255;not null;default:'';comment:split by space: sms voice email dingtalk wecom"`
	NotifyGroups     string `gorm:"size:255;not null;default:'';comment:split by space: 233 43"`
	NotifyRepeatNext int64  `gorm:"not null;default:0;comment:next timestamp to notify, get repeat settings from rule;index"`
	NotifyCurNumber  int32  `gorm:"not null;default:0"`
	TargetIdent      string `gorm:"size:191;not null;default:'';comment:target ident, also in tags"`
	TargetNote       string `gorm:"size:191;not null;default:'';comment:target note"`
	FirstTriggerTime int64
	TriggerTime      int64  `gorm:"not null;index"`
	TriggerValue     string `gorm:"type:text;not null"`
	Annotations      string `gorm:"type:text;not null;comment:annotations"`
	RuleConfig       string `gorm:"type:text;not null;comment:annotations"`
	Tags             string `gorm:"size:1024;not null;default:'';comment:merge data_tags rule_tags, split by ,,"`
	OriginalTags     string `gorm:"type:text;comment:labels key=val,,k2=v2"`
}

func (InitAlertCurEvent) TableName() string {
	return "alert_cur_event"
}

func (InitAlertCurEvent) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresAlertCurEvent struct {
	ID               uint64 `gorm:"primaryKey;NOT NULL;COMMENT:use alert_his_event.id"`
	Cate             string `gorm:"size:128;not null"`
	DatasourceID     int64  `gorm:"not null;default:0;comment:datasource id"`
	Cluster          string `gorm:"size:128;not null"`
	GroupID          uint64 `gorm:"not null;comment:busi group id of rule;index"`
	GroupName        string `gorm:"size:255;not null;default:'';comment:busi group name"`
	Hash             string `gorm:"size:64;not null;comment:rule_id + vector_pk;index"`
	RuleID           uint64 `gorm:"not null;index"`
	RuleName         string `gorm:"size:255;not null"`
	RuleNote         string `gorm:"size:2048;not null;default:'alert rule note'"`
	RuleProd         string `gorm:"size:255;not null;default:''"`
	RuleAlgo         string `gorm:"size:255;not null;default:''"`
	Severity         int16  `gorm:"type:smallint;not null;comment:0:Emergency 1:Warning 2:Notice"`
	PromForDuration  int32  `gorm:"not null;comment:prometheus for, unit:s"`
	PromQL           string `gorm:"size:8192;not null;comment:promql"`
	PromEvalInterval int32  `gorm:"not null;comment:evaluate interval"`
	Callbacks        string `gorm:"size:2048;not null;default:'';comment:split by space: http://a.com/api/x http://a.com/api/y"`
	RunbookURL       string `gorm:"size:255"`
	NotifyRecovered  int16  `gorm:"type:smallint;not null;comment:whether notify when recovery"`
	NotifyChannels   string `gorm:"size:255;not null;default:'';comment:split by space: sms voice email dingtalk wecom"`
	NotifyGroups     string `gorm:"size:255;not null;default:'';comment:split by space: 233 43"`
	NotifyRepeatNext int64  `gorm:"not null;default:0;comment:next timestamp to notify, get repeat settings from rule;index"`
	NotifyCurNumber  int32  `gorm:"not null;default:0"`
	TargetIdent      string `gorm:"size:191;not null;default:'';comment:target ident, also in tags"`
	TargetNote       string `gorm:"size:191;not null;default:'';comment:target note"`
	FirstTriggerTime int64
	TriggerTime      int64  `gorm:"not null;index"`
	TriggerValue     string `gorm:"type:text;not null"`
	Annotations      string `gorm:"type:text;not null;comment:annotations"`
	RuleConfig       string `gorm:"type:text;not null;comment:annotations"`
	Tags             string `gorm:"size:1024;not null;default:'';comment:merge data_tags rule_tags, split by ,,"`
	OriginalTags     string `gorm:"type:text;comment:labels key=val,,k2=v2"`
}

func (InitPostgresAlertCurEvent) TableName() string {
	return "alert_cur_event"
}

type InitAlertHisEvent struct {
	ID               uint64 `gorm:"primaryKey;autoIncrement"`
	IsRecovered      bool   `gorm:"type:tinyint(1);not null"`
	Cate             string `gorm:"size:128;not null"`
	DatasourceID     int64  `gorm:"not null;default:0;comment:datasource id"`
	Cluster          string `gorm:"size:128;not null"`
	GroupID          int64  `gorm:"not null;comment:busi group id of rule;index"`
	GroupName        string `gorm:"size:255;not null;default:'';comment:busi group name"`
	Hash             string `gorm:"size:64;not null;comment:rule_id + vector_pk;index"`
	RuleID           int64  `gorm:"not null;index"`
	RuleName         string `gorm:"size:255;not null"`
	RuleNote         string `gorm:"size:2048;not null;default:'alert rule note'"`
	RuleProd         string `gorm:"size:255;not null;default:''"`
	RuleAlgo         string `gorm:"size:255;not null;default:''"`
	Severity         int16  `gorm:"type:tinyint(1);not null;comment:0:Emergency 1:Warning 2:Notice"`
	PromForDuration  int32  `gorm:"not null;comment:prometheus for, unit:s"`
	PromQL           string `gorm:"size:8192;not null;comment:promql"`
	PromEvalInterval int32  `gorm:"not null;comment:evaluate interval"`
	Callbacks        string `gorm:"size:2048;not null;default:'';comment:split by space: http://a.com/api/x http://a.com/api/y"`
	RunbookURL       string `gorm:"size:255"`
	NotifyRecovered  bool   `gorm:"type:tinyint(1);not null;comment:whether notify when recovery"`
	NotifyChannels   string `gorm:"size:255;not null;default:'';comment:split by space: sms voice email dingtalk wecom"`
	NotifyGroups     string `gorm:"size:255;not null;default:'';comment:split by space: 233 43"`
	NotifyCurNumber  int32  `gorm:"not null;default:0"`
	TargetIdent      string `gorm:"size:191;not null;default:'';comment:target ident, also in tags"`
	TargetNote       string `gorm:"size:191;not null;default:'';comment:target note"`
	FirstTriggerTime int64
	TriggerTime      int64  `gorm:"not null;index"`
	TriggerValue     string `gorm:"type:text;not null"`
	RecoverTime      int64  `gorm:"not null;default:0"`
	LastEvalTime     int64  `gorm:"not null;default:0;comment:for time filter;index"`
	Tags             string `gorm:"size:1024;not null;default:'';comment:merge data_tags rule_tags, split by ,,"`
	OriginalTags     string `gorm:"type:text;comment:labels key=val,,k2=v2"`
	Annotations      string `gorm:"type:text;not null;comment:annotations"`
	RuleConfig       string `gorm:"type:text;not null;comment:annotations"`
}

func (InitAlertHisEvent) TableName() string {
	return "alert_his_event"
}

func (InitAlertHisEvent) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresAlertHisEvent struct {
	ID               uint64 `gorm:"primaryKey;autoIncrement"`
	IsRecovered      int16  `gorm:"type:smallint;not null"`
	Cate             string `gorm:"size:128;not null"`
	DatasourceID     int64  `gorm:"not null;default:0;comment:datasource id"`
	Cluster          string `gorm:"size:128;not null"`
	GroupID          int64  `gorm:"not null;comment:busi group id of rule;index"`
	GroupName        string `gorm:"size:255;not null;default:'';comment:busi group name"`
	Hash             string `gorm:"size:64;not null;comment:rule_id + vector_pk;index"`
	RuleID           int64  `gorm:"not null;index"`
	RuleName         string `gorm:"size:255;not null"`
	RuleNote         string `gorm:"size:2048;not null;default:'alert rule note'"`
	RuleProd         string `gorm:"size:255;not null;default:''"`
	RuleAlgo         string `gorm:"size:255;not null;default:''"`
	Severity         int16  `gorm:"type:smallint;not null;comment:0:Emergency 1:Warning 2:Notice"`
	PromForDuration  int32  `gorm:"not null;comment:prometheus for, unit:s"`
	PromQL           string `gorm:"size:8192;not null;comment:promql"`
	PromEvalInterval int32  `gorm:"not null;comment:evaluate interval"`
	Callbacks        string `gorm:"size:2048;not null;default:'';comment:split by space: http://a.com/api/x http://a.com/api/y"`
	RunbookURL       string `gorm:"size:255"`
	NotifyRecovered  int16  `gorm:"type:smallint;not null;comment:whether notify when recovery"`
	NotifyChannels   string `gorm:"size:255;not null;default:'';comment:split by space: sms voice email dingtalk wecom"`
	NotifyGroups     string `gorm:"size:255;not null;default:'';comment:split by space: 233 43"`
	NotifyCurNumber  int32  `gorm:"not null;default:0"`
	TargetIdent      string `gorm:"size:191;not null;default:'';comment:target ident, also in tags"`
	TargetNote       string `gorm:"size:191;not null;default:'';comment:target note"`
	FirstTriggerTime int64
	TriggerTime      int64  `gorm:"not null;index"`
	TriggerValue     string `gorm:"type:text;not null"`
	RecoverTime      int64  `gorm:"not null;default:0"`
	LastEvalTime     int64  `gorm:"not null;default:0;comment:for time filter;index"`
	Tags             string `gorm:"size:1024;not null;default:'';comment:merge data_tags rule_tags, split by ,,"`
	OriginalTags     string `gorm:"type:text;comment:labels key=val,,k2=v2"`
	Annotations      string `gorm:"type:text;not null;comment:annotations"`
	RuleConfig       string `gorm:"type:text;not null;comment:annotations"`
}

func (InitPostgresAlertHisEvent) TableName() string {
	return "alert_his_event"
}

type InitBoardBusiGroup struct {
	BusiGroupID int64 `primaryKey;gorm:"not null;default:0;comment:busi group id"`
	BoardID     int64 `primaryKey;gorm:"not null;default:0;comment:board id"`
}

func (InitBoardBusiGroup) TableName() string {
	return "board_busigroup"
}

func (InitBoardBusiGroup) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitBuiltinComponent struct {
	ID        int64  `gorm:"primaryKey;not null;autoIncrement;comment:unique identifier"`
	Ident     string `gorm:"size:191;not null;comment:identifier of component;index"`
	Logo      string `gorm:"size:191;not null;comment:logo of component"`
	Readme    string `gorm:"type:text;not null;comment:readme of component"`
	CreatedAt int64  `gorm:"not null;default:0;comment:create time"`
	CreatedBy string `gorm:"size:191;not null;default:'';comment:creator"`
	UpdatedAt int64  `gorm:"not null;default:0;comment:update time"`
	UpdatedBy string `gorm:"size:191;not null;default:'';comment:updater"`
}

func (InitBuiltinComponent) TableName() string {
	return "builtin_components"
}

func (InitBuiltinComponent) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitpostgresBuiltinPayload struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement;comment:unique identifier"`
	ComponentID uint64 `gorm:"not null;default:0;comment:component_id"`
	UUID        uint64 `gorm:"not null;comment:uuid of payload;index"`
	Type        string `gorm:"size:191;not null;comment:type of payload;index"`
	Component   string `gorm:"size:191;not null;comment:component of payload;index"`
	Cate        string `gorm:"size:191;not null;comment:category of payload;index"`
	Name        string `gorm:"size:191;not null;comment:name of payload;index"`
	Tags        string `gorm:"size:191;not null;default:'';comment:tags of payload"`
	Content     string `gorm:"type:TEXT;not null;comment:content of payload"`
	CreatedAt   int64  `gorm:"not null;default:0;comment:create time"`
	CreatedBy   string `gorm:"size:191;not null;default:'';comment:creator"`
	UpdatedAt   int64  `gorm:"not null;default:0;comment:update time"`
	UpdatedBy   string `gorm:"size:191;not null;default:'';comment:updater"`
}

func (InitpostgresBuiltinPayload) TableName() string {
	return "builtin_payloads"
}

type InitBuiltinPayload struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement;comment:unique identifier"`
	ComponentID uint64 `gorm:"not null;default:0;comment:component_id"`
	UUID        uint64 `gorm:"not null;comment:uuid of payload;index"`
	Type        string `gorm:"size:191;not null;comment:type of payload;index"`
	Component   string `gorm:"size:191;not null;comment:component of payload;index"`
	Cate        string `gorm:"size:191;not null;comment:category of payload;index"`
	Name        string `gorm:"size:191;not null;comment:name of payload;index"`
	Tags        string `gorm:"size:191;not null;default:'';comment:tags of payload"`
	Content     string `gorm:"type:longtext;not null;comment:content of payload"`
	CreatedAt   int64  `gorm:"not null;default:0;comment:create time"`
	CreatedBy   string `gorm:"size:191;not null;default:'';comment:creator"`
	UpdatedAt   int64  `gorm:"not null;default:0;comment:update time"`
	UpdatedBy   string `gorm:"size:191;not null;default:'';comment:updater"`
}

func (InitBuiltinPayload) TableName() string {
	return "builtin_payloads"
}

func (InitBuiltinPayload) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitNotificationRecord struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	EventID   uint64 `gorm:"not null;index:idx_evt"`
	SubID     uint64 `gorm:"not null"`
	Channel   string `gorm:"size:255;not null"`
	Status    int32  `gorm:"not null;default:0"`
	Target    string `gorm:"size:1024;not null"`
	Details   string `gorm:"size:2048"`
	CreatedAt int64  `gorm:"not null"`
}

func (InitNotificationRecord) TableName() string {
	return "notification_record"
}

func (InitNotificationRecord) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitTaskTpl struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	GroupID   int64  `gorm:"not null;comment:busi group id;index"`
	Title     string `gorm:"size:255;not null;default:''"`
	Account   string `gorm:"size:64;not null"`
	Batch     uint   `gorm:"not null;default:0"`
	Tolerance uint   `gorm:"not null;default:0"`
	Timeout   uint   `gorm:"not null;default:0"`
	Pause     string `gorm:"size:255;not null;default:''"`
	Script    string `gorm:"type:text;not null"`
	Args      string `gorm:"size:512;not null;default:''"`
	Tags      string `gorm:"size:255;not null;default:'';comment:split by space"`
	CreateAt  int64  `gorm:"not null;default:0"`
	CreateBy  string `gorm:"size:64;not null;default:''"`
	UpdateAt  int64  `gorm:"not null;default:0"`
	UpdateBy  string `gorm:"size:64;not null;default:''"`
}

func (InitTaskTpl) TableName() string {
	return "task_tpl"
}

func (InitTaskTpl) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitTaskTplHost struct {
	II   uint64 `gorm:"primaryKey;autoIncrement"`
	ID   uint64 `gorm:"not null;comment:task tpl id;index:idx_id_host"`
	Host string `gorm:"size:128;not null;comment:ip or hostname;index:idx_id_host"`
}

func (InitTaskTplHost) TableName() string {
	return "task_tpl_host"
}

func (InitTaskTplHost) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitTaskRecord struct {
	ID           uint64 `gorm:"primaryKey"`
	EventID      uint64 `gorm:"not null;default:0;comment:event id;index"`
	GroupID      uint64 `gorm:"not null;comment:busi group id;index:idx_group_id_create_at"`
	IbexAddress  string `gorm:"size:128;not null"`
	IbexAuthUser string `gorm:"size:128;not null;default:''"`
	IbexAuthPass string `gorm:"size:128;not null;default:''"`
	Title        string `gorm:"size:255;not null;default:''"`
	Account      string `gorm:"size:64;not null"`
	Batch        uint   `gorm:"not null;default:0"`
	Tolerance    uint   `gorm:"not null;default:0"`
	Timeout      uint   `gorm:"not null;default:0"`
	Pause        string `gorm:"size:255;not null;default:''"`
	Script       string `gorm:"type:text;not null"`
	Args         string `gorm:"size:512;not null;default:''"`
	CreateAt     int64  `gorm:"not null;default:0;index:idx_group_id_create_at"`
	CreateBy     string `gorm:"size:64;not null;default:'';index"`
}

func (InitTaskRecord) TableName() string {
	return "task_record"
}

func (InitTaskRecord) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitAlertingEngine struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement"`
	Instance      string `gorm:"size:128;not null;default:'';comment:instance identification, e.g. 10.9.0.9:9090"`
	DatasourceID  int64  `gorm:"not null;default:0;comment:datasource id"`
	EngineCluster string `gorm:"size:128;not null;default:'';comment:n9e-alert cluster"`
	Clock         int64  `gorm:"not null"`
}

func (InitAlertingEngine) TableName() string {
	return "alerting_engines"
}

func (InitAlertingEngine) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitDatasource struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement"`
	Name           string `gorm:"size:191;not null;default:'';uniqueIndex"`
	Description    string `gorm:"size:255;not null;default:''"`
	Category       string `gorm:"size:255;not null;default:''"`
	PluginID       uint   `gorm:"not null;default:0"`
	PluginType     string `gorm:"size:255;not null;default:''"`
	PluginTypeName string `gorm:"size:255;not null;default:''"`
	ClusterName    string `gorm:"size:255;not null;default:''"`
	Settings       string `gorm:"type:text;not null"`
	Status         string `gorm:"size:255;not null;default:''"`
	HTTP           string `gorm:"size:4096;not null;default:''"`
	Auth           string `gorm:"size:8192;not null;default:''"`
	IsDefault      bool   `gorm:"type:tinyint(1);not null;default:0"`
	CreatedAt      int64  `gorm:"not null;default:0"`
	CreatedBy      string `gorm:"size:64;not null;default:''"`
	UpdatedAt      int64  `gorm:"not null;default:0"`
	UpdatedBy      string `gorm:"size:64;not null;default:''"`
}

func (InitDatasource) TableName() string {
	return "datasource"
}

func (InitDatasource) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitPostgresDatasource struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement"`
	Name           string `gorm:"size:191;not null;default:'';uniqueIndex"`
	Description    string `gorm:"size:255;not null;default:''"`
	Category       string `gorm:"size:255;not null;default:''"`
	PluginID       uint   `gorm:"not null;default:0"`
	PluginType     string `gorm:"size:255;not null;default:''"`
	PluginTypeName string `gorm:"size:255;not null;default:''"`
	ClusterName    string `gorm:"size:255;not null;default:''"`
	Settings       string `gorm:"type:text;not null"`
	Status         string `gorm:"size:255;not null;default:''"`
	HTTP           string `gorm:"size:4096;not null;default:''"`
	Auth           string `gorm:"size:8192;not null;default:''"`
	IsDefault      bool   `gorm:"typr:boolean;not null;default:0"`
	CreatedAt      int64  `gorm:"not null;default:0"`
	CreatedBy      string `gorm:"size:64;not null;default:''"`
	UpdatedAt      int64  `gorm:"not null;default:0"`
	UpdatedBy      string `gorm:"size:64;not null;default:''"`
}

func (InitPostgresDatasource) TableName() string {
	return "datasource"
}

type InitBuiltinCate struct {
	ID     uint64 `gorm:"primaryKey;autoIncrement"`
	Name   string `gorm:"size:191;not null"`
	UserID int64  `gorm:"not null;default:0"`
}

func (InitBuiltinCate) TableName() string {
	return "builtin_cate"
}

func (InitBuiltinCate) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitNotifyTpl struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	Channel  string `gorm:"size:32;not null;uniqueIndex"`
	Name     string `gorm:"size:255;not null"`
	Content  string `gorm:"type:text;not null"`
	CreateAt int64  `gorm:"not null;default:0"`
	CreateBy string `gorm:"size:64;not null;default:''"`
	UpdateAt int64  `gorm:"not null;default:0"`
	UpdateBy string `gorm:"size:64;not null;default:''"`
}

func (InitNotifyTpl) TableName() string {
	return "notify_tpl"
}

func (InitNotifyTpl) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitSSOConfig struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	Name     string `gorm:"size:191;not null;uniqueIndex"`
	Content  string `gorm:"type:text;not null"`
	UpdateAt int64  `gorm:"not null;default:0"`
}

func (InitSSOConfig) TableName() string {
	return "sso_config"
}

func (InitSSOConfig) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitESIndexPattern struct {
	ID                     uint64 `gorm:"primaryKey;autoIncrement"`
	DatasourceID           int64  `gorm:"not null;default:0;comment:datasource id;uniqueIndex:idx_datasource_name"`
	Name                   string `gorm:"size:191;not null;uniqueIndex:idx_datasource_name"`
	TimeField              string `gorm:"size:128;not null;default:'@timestamp'"`
	AllowHideSystemIndices bool   `gorm:"type:tinyint(1);not null;default:0"`
	FieldsFormat           string `gorm:"size:4096;not null;default:''"`
	CreateAt               int64  `gorm:"default:0"`
	CreateBy               string `gorm:"size:64;default:''"`
	UpdateAt               int64  `gorm:"default:0"`
	UpdateBy               string `gorm:"size:64;default:''"`
}

func (InitESIndexPattern) TableName() string {
	return "es_index_pattern"
}

func (InitESIndexPattern) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitSqliteESIndexPattern struct {
	ID                     uint64 `gorm:"primaryKey;autoIncrement"`
	DatasourceID           int64  `gorm:"not null;default:0;comment:datasource id;uniqueIndex:idx_datasource"`
	Name                   string `gorm:"size:191;not null;uniqueIndex:idx_name"`
	TimeField              string `gorm:"size:128;not null;default:'@timestamp'"`
	AllowHideSystemIndices bool   `gorm:"type:tinyint(1);not null;default:0"`
	FieldsFormat           string `gorm:"size:4096;not null;default:''"`
	CreateAt               int64  `gorm:"default:0"`
	CreateBy               string `gorm:"size:64;default:''"`
	UpdateAt               int64  `gorm:"default:0"`
	UpdateBy               string `gorm:"size:64;default:''"`
}

func (InitSqliteESIndexPattern) TableName() string {
	return "es_index_pattern"
}

type InitPostgresESIndexPattern struct {
	ID                     uint64 `gorm:"primaryKey;autoIncrement"`
	DatasourceID           int64  `gorm:"not null;default:0;comment:datasource id;uniqueIndex:idx_datasource_name"`
	Name                   string `gorm:"size:191;not null;uniqueIndex:idx_datasource_name"`
	TimeField              string `gorm:"size:128;not null;default:'@timestamp'"`
	AllowHideSystemIndices int16  `gorm:"type:smallint;not null;default:0"`
	FieldsFormat           string `gorm:"size:4096;not null;default:''"`
	CreateAt               int64  `gorm:"default:0"`
	CreateBy               string `gorm:"size:64;default:''"`
	UpdateAt               int64  `gorm:"default:0"`
	UpdateBy               string `gorm:"size:64;default:''"`
}

func (InitPostgresESIndexPattern) TableName() string {
	return "es_index_pattern"
}

type InitBuiltinMetric struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement;comment:unique identifier"`
	Collector  string `gorm:"size:191;not null;comment:type of collector;index:idx_collector;uniqueIndex:idx_collector_typ_name"`
	Typ        string `gorm:"size:191;not null;comment:type of metric;index:idx_typ;uniqueIndex:idx_collector_typ_name"`
	Name       string `gorm:"size:191;not null;comment:name of metric;index:idx_name;uniqueIndex:idx_collector_typ_name"`
	Unit       string `gorm:"size:191;not null;comment:unit of metric"`
	Lang       string `gorm:"size:191;not null;default:'';comment:language of metric;index:idx_lang;uniqueIndex:idx_collector_typ_name"`
	Note       string `gorm:"size:4096;not null;comment:description of metric in Chinese"`
	Expression string `gorm:"size:4096;not null;comment:expression of metric"`
	CreatedAt  int64  `gorm:"not null;default:0;comment:create time"`
	CreatedBy  string `gorm:"size:191;not null;default:'';comment:creator"`
	UpdatedAt  int64  `gorm:"not null;default:0;comment:update time"`
	UpdatedBy  string `gorm:"size:191;not null;default:'';comment:updater"`
	UUID       int64  `gorm:"not null;default:0;comment:'uuid'"`
}

func (InitBuiltinMetric) TableName() string {
	return "builtin_metrics"
}

func (InitBuiltinMetric) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitSqliteBuiltinMetric struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement;comment:unique identifier"`
	Collector  string `gorm:"size:191;not null;comment:type of collector;index:idx_collector;uniqueIndex:idx_collector_typ_name"`
	Typ        string `gorm:"size:191;not null;comment:type of metric;index:idx_typ;uniqueIndex:idx_collector_typ_name"`
	Name       string `gorm:"size:191;not null;comment:name of metric;index:idx_name_sqlite;uniqueIndex:idx_collector_typ_name"`
	Unit       string `gorm:"size:191;not null;comment:unit of metric"`
	Lang       string `gorm:"size:191;not null;default:'';comment:language of metric;index:idx_lang;uniqueIndex:idx_collector_typ_name"`
	Note       string `gorm:"size:4096;not null;comment:description of metric in Chinese"`
	Expression string `gorm:"size:4096;not null;comment:expression of metric"`
	CreatedAt  int64  `gorm:"not null;default:0;comment:create time"`
	CreatedBy  string `gorm:"size:191;not null;default:'';comment:creator"`
	UpdatedAt  int64  `gorm:"not null;default:0;comment:update time"`
	UpdatedBy  string `gorm:"size:191;not null;default:'';comment:updater"`
	UUID       int64  `gorm:"not null;default:0;comment:'uuid'"`
}

func (InitSqliteBuiltinMetric) TableName() string {
	return "builtin_metrics"
}

type InitMetricFilter struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement;comment:unique identifier"`
	Name       string `gorm:"size:191;not null;comment:name of metric filter;index:idx_name"`
	Configs    string `gorm:"size:4096;not null;comment:configuration of metric filter"`
	GroupsPerm string `gorm:"type:text"`
	CreateAt   int64  `gorm:"not null;default:0;comment:create time"`
	CreateBy   string `gorm:"size:191;not null;default:'';comment:creator"`
	UpdateAt   int64  `gorm:"not null;default:0;comment:update time"`
	UpdateBy   string `gorm:"size:191;not null;default:'';comment:updater"`
}

func (InitMetricFilter) TableName() string {
	return "metric_filter"
}

func (InitMetricFilter) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitSqliteMetricFilter struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement;comment:unique identifier"`
	Name       string `gorm:"size:191;not null;comment:name of metric filter;index:idx_name_metric_filter_sqlite"`
	Configs    string `gorm:"size:4096;not null;comment:configuration of metric filter"`
	GroupsPerm string `gorm:"type:text"`
	CreateAt   int64  `gorm:"not null;default:0;comment:create time"`
	CreateBy   string `gorm:"size:191;not null;default:'';comment:creator"`
	UpdateAt   int64  `gorm:"not null;default:0;comment:update time"`
	UpdateBy   string `gorm:"size:191;not null;default:'';comment:updater"`
}

func (InitSqliteMetricFilter) TableName() string {
	return "metric_filter"
}

type InitTargetBusiGroup struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	TargetIdent string `gorm:"size:191;not null;uniqueIndex:idx_target_group"`
	GroupID     uint64 `gorm:"not null;uniqueIndex:idx_target_group"`
	UpdateAt    int64  `gorm:"not null"`
}

func (InitTargetBusiGroup) TableName() string {
	return "target_busi_group"
}

func (InitTargetBusiGroup) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitTaskMeta struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	Title     string    `gorm:"size:255;not null;default:''"`
	Account   string    `gorm:"size:64;not null"`
	Batch     uint      `gorm:"not null;default:0"`
	Tolerance uint      `gorm:"not null;default:0"`
	Timeout   uint      `gorm:"not null;default:0"`
	Pause     string    `gorm:"size:255;not null;default:''"`
	Script    string    `gorm:"type:text;not null"`
	Args      string    `gorm:"size:512;not null;default:''"`
	Stdin     string    `gorm:"size:1024;not null;default:''"`
	Creator   string    `gorm:"size:64;not null;default:'';index"`
	Created   time.Time `gorm:"column:created;not null;default:CURRENT_TIMESTAMP;type:timestamp;index" json:"created"`
}

func (InitTaskMeta) TableName() string {
	return "task_meta"
}

func (InitTaskMeta) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitTaskAction struct {
	ID     uint64 `gorm:"primaryKey"`
	Action string `gorm:"size:32;not null"`
	Clock  int64  `gorm:"not null;default:0"`
}

func (InitTaskAction) TableName() string {
	return "task_action"
}

func (InitTaskAction) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitTaskScheduler struct {
	ID        uint64 `gorm:"primaryKey;index"`
	Scheduler string `gorm:"size:128;not null;default:'';index"`
}

func (InitTaskScheduler) TableName() string {
	return "task_scheduler"
}

func (InitTaskScheduler) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitTaskSchedulerHealth struct {
	Scheduler string `gorm:"size:128;not null;uniqueIndex"`
	Clock     int64  `gorm:"not null;index"`
}

func (InitTaskSchedulerHealth) TableName() string {
	return "task_scheduler_health"
}

func (InitTaskSchedulerHealth) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitTaskHostDoing struct {
	ID     uint64 `gorm:"primaryKey;index"`
	Host   string `gorm:"size:128;not null;index"`
	Clock  int64  `gorm:"not null;default:0"`
	Action string `gorm:"size:16;not null"`
}

func (InitTaskHostDoing) TableName() string {
	return "task_host_doing"
}

func (InitTaskHostDoing) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitTaskHost struct {
	II     uint64 `gorm:"primaryKey;autoIncrement"`
	ID     uint64 `gorm:"not null;uniqueIndex:id_host"`
	Host   string `gorm:"size:128;not null;uniqueIndex:id_host"`
	Status string `gorm:"size:32;not null"`
	Stdout string `gorm:"type:text"`
	Stderr string `gorm:"type:text"`
}

func (InitTaskHost) TableName() string {
	return "task_host_0"
}

func (InitTaskHost) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

type InitSqliteTaskHost struct {
	II     uint64 `gorm:"primaryKey;autoIncrement"`
	ID     uint64 `gorm:"not null;"`
	Host   string `gorm:"size:128;not null;"`
	Status string `gorm:"size:32;not null"`
	Stdout string `gorm:"type:text"`
	Stderr string `gorm:"type:text"`
}

func (InitSqliteTaskHost) TableName() string {
	return "task_host_0"
}

func DataBaseInit(c DBConfig, db *gorm.DB) error {
	switch strings.ToLower(c.DBType) {
	case "mysql":
		return mysqlDataBaseInit(db)
	case "postgres":
		return postgresDataBaseInit(db)
	case "sqlite":
		return sqliteDataBaseInit(db)
	default:
		return fmt.Errorf("unsupported database type: %s", c.DBType)
	}
}

func sqliteDataBaseInit(db *gorm.DB) error {
	dts := []interface{}{
		&InitTaskMeta{},
		&InitTaskAction{},
		&InitTaskScheduler{},
		&InitTaskSchedulerHealth{},
		&InitTaskHostDoing{},
		&InitSqliteTaskHost{},
		&InitBoardBusiGroup{},
		&InitBuiltinComponent{},
		&InitBuiltinPayload{},
		&InitNotificationRecord{},
		&InitTaskTpl{},
		&InitTaskTplHost{},
		&InitTaskRecord{},
		&InitAlertingEngine{},
		&InitDatasource{},
		&InitBuiltinCate{},
		&InitNotifyTpl{},
		&InitSSOConfig{},
		&InitSqliteESIndexPattern{},
		&InitSqliteBuiltinMetric{},
		&InitSqliteMetricFilter{},
		&InitTargetBusiGroup{},
		&InitAlertAggrView{},
		&InitAlertCurEvent{},
		&InitAlertHisEvent{},
		&InitAlertMute{},
		&InitAlertSubscribe{},
		&InitTarget{},
		&InitMetricView{},
		&InitRecordingRule{},
		&InitUser{},
		&InitUserGroup{},
		&InitUserGroupMember{},
		&InitConfig{},
		&InitRole{},
		&InitRoleOperation{},
		&InitBusiGroup{},
		&InitBusiGroupMember{},
		&InitBoard{},
		&InitBoardPayload{},
		&InitDashboard{},
		&InitChartGroup{},
		&InitChart{},
		&InitChartShare{},
		&InitAlertRule{}}

	for _, dt := range dts {
		err := db.AutoMigrate(dt)
		if err != nil {
			fmt.Printf("sqliteDataBaseInit AutoMigrate error: %v\n", err)
			return err
		}
	}

	for i := 1; i <= 99; i++ {
		tableName := "task_host_" + strconv.Itoa(i)
		err := db.Table(tableName).AutoMigrate(&InitSqliteTaskHost{})
		if err != nil {
			return err
		}
	}

	roleOperations := []InitRoleOperation{
		{RoleName: "Guest", Operation: "/metric/explorer"},
		{RoleName: "Guest", Operation: "/object/explorer"},
		{RoleName: "Guest", Operation: "/log/explorer"},
		{RoleName: "Guest", Operation: "/trace/explorer"},
		{RoleName: "Guest", Operation: "/help/version"},
		{RoleName: "Guest", Operation: "/help/contact"},
		{RoleName: "Standard", Operation: "/metric/explorer"},
		{RoleName: "Standard", Operation: "/object/explorer"},
		{RoleName: "Standard", Operation: "/log/explorer"},
		{RoleName: "Standard", Operation: "/trace/explorer"},
		{RoleName: "Standard", Operation: "/help/version"},
		{RoleName: "Standard", Operation: "/help/contact"},
		{RoleName: "Standard", Operation: "/help/servers"},
		{RoleName: "Standard", Operation: "/help/migrate"},
		{RoleName: "Standard", Operation: "/alert-rules-built-in"},
		{RoleName: "Standard", Operation: "/dashboards-built-in"},
		{RoleName: "Standard", Operation: "/trace/dependencies"},
		{RoleName: "Admin", Operation: "/help/source"},
		{RoleName: "Admin", Operation: "/help/sso"},
		{RoleName: "Admin", Operation: "/help/notification-tpls"},
		{RoleName: "Admin", Operation: "/help/notification-settings"},
		{RoleName: "Standard", Operation: "/users"},
		{RoleName: "Standard", Operation: "/user-groups"},
		{RoleName: "Standard", Operation: "/user-groups/add"},
		{RoleName: "Standard", Operation: "/user-groups/put"},
		{RoleName: "Standard", Operation: "/user-groups/del"},
		{RoleName: "Standard", Operation: "/busi-groups"},
		{RoleName: "Standard", Operation: "/busi-groups/add"},
		{RoleName: "Standard", Operation: "/busi-groups/put"},
		{RoleName: "Standard", Operation: "/busi-groups/del"},
		{RoleName: "Standard", Operation: "/targets"},
		{RoleName: "Standard", Operation: "/targets/add"},
		{RoleName: "Standard", Operation: "/targets/put"},
		{RoleName: "Standard", Operation: "/targets/del"},
		{RoleName: "Standard", Operation: "/dashboards"},
		{RoleName: "Standard", Operation: "/dashboards/add"},
		{RoleName: "Standard", Operation: "/dashboards/put"},
		{RoleName: "Standard", Operation: "/dashboards/del"},
		{RoleName: "Standard", Operation: "/alert-rules"},
		{RoleName: "Standard", Operation: "/alert-rules/add"},
		{RoleName: "Standard", Operation: "/alert-rules/put"},
		{RoleName: "Standard", Operation: "/alert-rules/del"},
		{RoleName: "Standard", Operation: "/alert-mutes"},
		{RoleName: "Standard", Operation: "/alert-mutes/add"},
		{RoleName: "Standard", Operation: "/alert-mutes/del"},
		{RoleName: "Standard", Operation: "/alert-subscribes"},
		{RoleName: "Standard", Operation: "/alert-subscribes/add"},
		{RoleName: "Standard", Operation: "/alert-subscribes/put"},
		{RoleName: "Standard", Operation: "/alert-subscribes/del"},
		{RoleName: "Standard", Operation: "/alert-cur-events"},
		{RoleName: "Standard", Operation: "/alert-cur-events/del"},
		{RoleName: "Standard", Operation: "/alert-his-events"},
		{RoleName: "Standard", Operation: "/job-tpls"},
		{RoleName: "Standard", Operation: "/job-tpls/add"},
		{RoleName: "Standard", Operation: "/job-tpls/put"},
		{RoleName: "Standard", Operation: "/job-tpls/del"},
		{RoleName: "Standard", Operation: "/job-tasks"},
		{RoleName: "Standard", Operation: "/job-tasks/add"},
		{RoleName: "Standard", Operation: "/job-tasks/put"},
		{RoleName: "Standard", Operation: "/recording-rules"},
		{RoleName: "Standard", Operation: "/recording-rules/add"},
		{RoleName: "Standard", Operation: "/recording-rules/put"},
		{RoleName: "Standard", Operation: "/recording-rules/del"},
	}

	entries := []struct {
		name  string
		entry interface{}
	}{
		{
			name:  "InitUser",
			entry: &InitUser{ID: 1, Username: "root", Nickname: "超管", Password: "root.2020", Roles: "Admin", CreateAt: time.Now().Unix(), CreateBy: "system", UpdateAt: time.Now().Unix(), UpdateBy: "system"},
		},
		{
			name:  "InitUserGroup",
			entry: &InitUserGroup{ID: 1, Name: "demo-root-group", CreateAt: time.Now().Unix(), CreateBy: "root", UpdateAt: time.Now().Unix(), UpdateBy: "root"},
		},
		{
			name:  "InitUserGroupMember",
			entry: &InitUserGroupMember{GroupID: 1, UserID: 1},
		},
		{
			name:  "InitRole",
			entry: &InitRole{Name: "Admin", Note: "Administrator role"},
		},
		{
			name:  "InitRole",
			entry: &InitRole{Name: "Standard", Note: "Ordinary user role"},
		},
		{
			name:  "InitRole",
			entry: &InitRole{Name: "Guest", Note: "Readonly user role"},
		},
		{
			name:  "InitBusiGroup",
			entry: &InitBusiGroup{ID: 1, Name: "Default Busi Group", CreateAt: time.Now().Unix(), CreateBy: "root", UpdateAt: time.Now().Unix(), UpdateBy: "root"},
		},
		{
			name:  "InitBusiGroupMember",
			entry: &InitBusiGroupMember{BusiGroupID: 1, UserGroupID: 1, PermFlag: "rw"},
		},
		{
			name:  "InitMetricView",
			entry: &InitMetricView{Name: "Host View", Cate: false, Configs: `{"filters":[{"oper":"=","label":"__name__","value":"cpu_usage_idle"}],"dynamicLabels":[],"dimensionLabels":[{"label":"ident","value":""}]}`},
		},
		{
			name:  "InitAlertAggrView",
			entry: &InitAlertAggrView{Name: "By BusiGroup, Severity", Rule: "field:group_name::field:severity", Cate: false},
		},
		{
			name:  "InitAlertAggrView",
			entry: &InitAlertAggrView{Name: "By RuleName", Rule: "field:rule_name", Cate: false},
		},
	}

	for _, roleOperation := range roleOperations {
		err := db.Create(&roleOperation).Error
		if err != nil {
			logger.Errorf("[sqlite database init]create role operation error: %v", err)
		}
	}

	for _, entry := range entries {
		if err := db.Create(entry.entry).Error; err != nil {
			logger.Errorf("[sqlite database init]create %s error: %v", entry.name, err)
		}
	}

	return nil
}

func mysqlDataBaseInit(db *gorm.DB) error {
	dts := []interface{}{
		&InitTaskMeta{},
		&InitTaskAction{},
		&InitTaskScheduler{},
		&InitTaskSchedulerHealth{},
		&InitTaskHostDoing{},
		&InitTaskHost{},
		&InitBoardBusiGroup{},
		&InitBuiltinComponent{},
		&InitBuiltinPayload{},
		&InitNotificationRecord{},
		&InitTaskTpl{},
		&InitTaskTplHost{},
		&InitTaskRecord{},
		&InitAlertingEngine{},
		&InitDatasource{},
		&InitBuiltinCate{},
		&InitNotifyTpl{},
		&InitSSOConfig{},
		&InitESIndexPattern{},
		&InitBuiltinMetric{},
		&InitMetricFilter{},
		&InitTargetBusiGroup{},
		&InitAlertAggrView{},
		&InitAlertCurEvent{},
		&InitAlertHisEvent{},
		&InitAlertMute{},
		&InitAlertSubscribe{},
		&InitTarget{},
		&InitMetricView{},
		&InitRecordingRule{},
		&InitUser{},
		&InitUserGroup{},
		&InitUserGroupMember{},
		&InitConfig{},
		&InitRole{},
		&InitRoleOperation{},
		&InitBusiGroup{},
		&InitBusiGroupMember{},
		&InitBoard{},
		&InitBoardPayload{},
		&InitDashboard{},
		&InitChartGroup{},
		&InitChart{},
		&InitChartShare{},
		&InitAlertRule{}}

	for _, dt := range dts {
		err := db.AutoMigrate(dt)
		if err != nil {
			fmt.Printf("mysqlDataBaseInit AutoMigrate error: %v\n", err)
			return err
		}
	}

	for i := 1; i <= 99; i++ {
		tableName := "task_host_" + strconv.Itoa(i)
		err := db.Table(tableName).AutoMigrate(&InitTaskHost{})
		if err != nil {
			return err
		}
	}

	roleOperations := []InitRoleOperation{
		{RoleName: "Guest", Operation: "/metric/explorer"},
		{RoleName: "Guest", Operation: "/object/explorer"},
		{RoleName: "Guest", Operation: "/log/explorer"},
		{RoleName: "Guest", Operation: "/trace/explorer"},
		{RoleName: "Guest", Operation: "/help/version"},
		{RoleName: "Guest", Operation: "/help/contact"},
		{RoleName: "Standard", Operation: "/metric/explorer"},
		{RoleName: "Standard", Operation: "/object/explorer"},
		{RoleName: "Standard", Operation: "/log/explorer"},
		{RoleName: "Standard", Operation: "/trace/explorer"},
		{RoleName: "Standard", Operation: "/help/version"},
		{RoleName: "Standard", Operation: "/help/contact"},
		{RoleName: "Standard", Operation: "/help/servers"},
		{RoleName: "Standard", Operation: "/help/migrate"},
		{RoleName: "Standard", Operation: "/alert-rules-built-in"},
		{RoleName: "Standard", Operation: "/dashboards-built-in"},
		{RoleName: "Standard", Operation: "/trace/dependencies"},
		{RoleName: "Admin", Operation: "/help/source"},
		{RoleName: "Admin", Operation: "/help/sso"},
		{RoleName: "Admin", Operation: "/help/notification-tpls"},
		{RoleName: "Admin", Operation: "/help/notification-settings"},
		{RoleName: "Standard", Operation: "/users"},
		{RoleName: "Standard", Operation: "/user-groups"},
		{RoleName: "Standard", Operation: "/user-groups/add"},
		{RoleName: "Standard", Operation: "/user-groups/put"},
		{RoleName: "Standard", Operation: "/user-groups/del"},
		{RoleName: "Standard", Operation: "/busi-groups"},
		{RoleName: "Standard", Operation: "/busi-groups/add"},
		{RoleName: "Standard", Operation: "/busi-groups/put"},
		{RoleName: "Standard", Operation: "/busi-groups/del"},
		{RoleName: "Standard", Operation: "/targets"},
		{RoleName: "Standard", Operation: "/targets/add"},
		{RoleName: "Standard", Operation: "/targets/put"},
		{RoleName: "Standard", Operation: "/targets/del"},
		{RoleName: "Standard", Operation: "/dashboards"},
		{RoleName: "Standard", Operation: "/dashboards/add"},
		{RoleName: "Standard", Operation: "/dashboards/put"},
		{RoleName: "Standard", Operation: "/dashboards/del"},
		{RoleName: "Standard", Operation: "/alert-rules"},
		{RoleName: "Standard", Operation: "/alert-rules/add"},
		{RoleName: "Standard", Operation: "/alert-rules/put"},
		{RoleName: "Standard", Operation: "/alert-rules/del"},
		{RoleName: "Standard", Operation: "/alert-mutes"},
		{RoleName: "Standard", Operation: "/alert-mutes/add"},
		{RoleName: "Standard", Operation: "/alert-mutes/del"},
		{RoleName: "Standard", Operation: "/alert-subscribes"},
		{RoleName: "Standard", Operation: "/alert-subscribes/add"},
		{RoleName: "Standard", Operation: "/alert-subscribes/put"},
		{RoleName: "Standard", Operation: "/alert-subscribes/del"},
		{RoleName: "Standard", Operation: "/alert-cur-events"},
		{RoleName: "Standard", Operation: "/alert-cur-events/del"},
		{RoleName: "Standard", Operation: "/alert-his-events"},
		{RoleName: "Standard", Operation: "/job-tpls"},
		{RoleName: "Standard", Operation: "/job-tpls/add"},
		{RoleName: "Standard", Operation: "/job-tpls/put"},
		{RoleName: "Standard", Operation: "/job-tpls/del"},
		{RoleName: "Standard", Operation: "/job-tasks"},
		{RoleName: "Standard", Operation: "/job-tasks/add"},
		{RoleName: "Standard", Operation: "/job-tasks/put"},
		{RoleName: "Standard", Operation: "/recording-rules"},
		{RoleName: "Standard", Operation: "/recording-rules/add"},
		{RoleName: "Standard", Operation: "/recording-rules/put"},
		{RoleName: "Standard", Operation: "/recording-rules/del"},
	}

	entries := []struct {
		name  string
		entry interface{}
	}{
		{
			name:  "InitUser",
			entry: &InitUser{ID: 1, Username: "root", Nickname: "超管", Password: "root.2020", Roles: "Admin", CreateAt: time.Now().Unix(), CreateBy: "system", UpdateAt: time.Now().Unix(), UpdateBy: "system"},
		},
		{
			name:  "InitUserGroup",
			entry: &InitUserGroup{ID: 1, Name: "demo-root-group", CreateAt: time.Now().Unix(), CreateBy: "root", UpdateAt: time.Now().Unix(), UpdateBy: "root"},
		},
		{
			name:  "InitUserGroupMember",
			entry: &InitUserGroupMember{GroupID: 1, UserID: 1},
		},
		{
			name:  "InitRole",
			entry: &InitRole{Name: "Admin", Note: "Administrator role"},
		},
		{
			name:  "InitRole",
			entry: &InitRole{Name: "Standard", Note: "Ordinary user role"},
		},
		{
			name:  "InitRole",
			entry: &InitRole{Name: "Guest", Note: "Readonly user role"},
		},
		{
			name:  "InitBusiGroup",
			entry: &InitBusiGroup{ID: 1, Name: "Default Busi Group", CreateAt: time.Now().Unix(), CreateBy: "root", UpdateAt: time.Now().Unix(), UpdateBy: "root"},
		},
		{
			name:  "InitBusiGroupMember",
			entry: &InitBusiGroupMember{BusiGroupID: 1, UserGroupID: 1, PermFlag: "rw"},
		},
		{
			name:  "InitMetricView",
			entry: &InitMetricView{Name: "Host View", Cate: false, Configs: `{"filters":[{"oper":"=","label":"__name__","value":"cpu_usage_idle"}],"dynamicLabels":[],"dimensionLabels":[{"label":"ident","value":""}]}`},
		},
		{
			name:  "InitAlertAggrView",
			entry: &InitAlertAggrView{Name: "By BusiGroup, Severity", Rule: "field:group_name::field:severity", Cate: false},
		},
		{
			name:  "InitAlertAggrView",
			entry: &InitAlertAggrView{Name: "By RuleName", Rule: "field:rule_name", Cate: false},
		},
	}

	for _, roleOperation := range roleOperations {
		err := db.Create(&roleOperation).Error
		if err != nil {
			logger.Errorf("[mysql database init]create role operation error: %v", err)
		}
	}

	for _, entry := range entries {
		if err := db.Create(entry.entry).Error; err != nil {
			logger.Errorf("[mysql database init]create %s error: %v", entry.name, err)
		}
	}

	return nil
}

func postgresDataBaseInit(db *gorm.DB) error {
	dts := []interface{}{
		&InitTaskMeta{},
		&InitTaskAction{},
		&InitTaskScheduler{},
		&InitTaskSchedulerHealth{},
		&InitTaskHostDoing{},
		&InitTaskHost{},
		&InitBoardBusiGroup{},
		&InitBuiltinComponent{},
		&InitpostgresBuiltinPayload{},
		&InitNotificationRecord{},
		&InitTaskTpl{},
		&InitTaskTplHost{},
		&InitTaskRecord{},
		&InitAlertingEngine{},
		&InitPostgresDatasource{},
		&InitBuiltinCate{},
		&InitNotifyTpl{},
		&InitSSOConfig{},
		&InitPostgresESIndexPattern{},
		&InitBuiltinMetric{},
		&InitMetricFilter{},
		&InitTargetBusiGroup{},
		&InitPostgresAlertAggrView{},
		&InitPostgresAlertCurEvent{},
		&InitPostgresAlertHisEvent{},
		&InitPostgresAlertMute{},
		&InitPostgresAlertSubscribe{},
		&InitTarget{},
		&InitPostgresMetricView{},
		&InitPostgresRecordingRule{},
		&InitPostgresUser{},
		&InitUserGroup{},
		&InitUserGroupMember{},
		&InitPostgresConfig{},
		&InitRole{},
		&InitRoleOperation{},
		&InitPostgresBusiGroup{},
		&InitBusiGroupMember{},
		&InitPostgresBoard{},
		&InitPostgresBoardPayload{},
		&InitDashboard{},
		&InitChartGroup{},
		&InitChart{},
		&InitChartShare{},
		&InitPostgresAlertRule{}}

	for _, dt := range dts {
		err := db.AutoMigrate(dt)
		if err != nil {
			fmt.Printf("postgresDataBaseInit AutoMigrate error: %v\n", err)
			return err
		}
	}

	for i := 1; i <= 99; i++ {
		tableName := "task_host_" + strconv.Itoa(i)
		err := db.Table(tableName).AutoMigrate(&InitTaskHost{})
		if err != nil {
			return err
		}
	}

	roleOperations := []InitRoleOperation{
		{RoleName: "Guest", Operation: "/metric/explorer"},
		{RoleName: "Guest", Operation: "/object/explorer"},
		{RoleName: "Guest", Operation: "/log/explorer"},
		{RoleName: "Guest", Operation: "/trace/explorer"},
		{RoleName: "Guest", Operation: "/help/version"},
		{RoleName: "Guest", Operation: "/help/contact"},
		{RoleName: "Standard", Operation: "/metric/explorer"},
		{RoleName: "Standard", Operation: "/object/explorer"},
		{RoleName: "Standard", Operation: "/log/explorer"},
		{RoleName: "Standard", Operation: "/trace/explorer"},
		{RoleName: "Standard", Operation: "/help/version"},
		{RoleName: "Standard", Operation: "/help/contact"},
		{RoleName: "Standard", Operation: "/help/servers"},
		{RoleName: "Standard", Operation: "/help/migrate"},
		{RoleName: "Standard", Operation: "/alert-rules-built-in"},
		{RoleName: "Standard", Operation: "/dashboards-built-in"},
		{RoleName: "Standard", Operation: "/trace/dependencies"},
		{RoleName: "Admin", Operation: "/help/source"},
		{RoleName: "Admin", Operation: "/help/sso"},
		{RoleName: "Admin", Operation: "/help/notification-tpls"},
		{RoleName: "Admin", Operation: "/help/notification-settings"},
		{RoleName: "Standard", Operation: "/users"},
		{RoleName: "Standard", Operation: "/user-groups"},
		{RoleName: "Standard", Operation: "/user-groups/add"},
		{RoleName: "Standard", Operation: "/user-groups/put"},
		{RoleName: "Standard", Operation: "/user-groups/del"},
		{RoleName: "Standard", Operation: "/busi-groups"},
		{RoleName: "Standard", Operation: "/busi-groups/add"},
		{RoleName: "Standard", Operation: "/busi-groups/put"},
		{RoleName: "Standard", Operation: "/busi-groups/del"},
		{RoleName: "Standard", Operation: "/targets"},
		{RoleName: "Standard", Operation: "/targets/add"},
		{RoleName: "Standard", Operation: "/targets/put"},
		{RoleName: "Standard", Operation: "/targets/del"},
		{RoleName: "Standard", Operation: "/dashboards"},
		{RoleName: "Standard", Operation: "/dashboards/add"},
		{RoleName: "Standard", Operation: "/dashboards/put"},
		{RoleName: "Standard", Operation: "/dashboards/del"},
		{RoleName: "Standard", Operation: "/alert-rules"},
		{RoleName: "Standard", Operation: "/alert-rules/add"},
		{RoleName: "Standard", Operation: "/alert-rules/put"},
		{RoleName: "Standard", Operation: "/alert-rules/del"},
		{RoleName: "Standard", Operation: "/alert-mutes"},
		{RoleName: "Standard", Operation: "/alert-mutes/add"},
		{RoleName: "Standard", Operation: "/alert-mutes/del"},
		{RoleName: "Standard", Operation: "/alert-subscribes"},
		{RoleName: "Standard", Operation: "/alert-subscribes/add"},
		{RoleName: "Standard", Operation: "/alert-subscribes/put"},
		{RoleName: "Standard", Operation: "/alert-subscribes/del"},
		{RoleName: "Standard", Operation: "/alert-cur-events"},
		{RoleName: "Standard", Operation: "/alert-cur-events/del"},
		{RoleName: "Standard", Operation: "/alert-his-events"},
		{RoleName: "Standard", Operation: "/job-tpls"},
		{RoleName: "Standard", Operation: "/job-tpls/add"},
		{RoleName: "Standard", Operation: "/job-tpls/put"},
		{RoleName: "Standard", Operation: "/job-tpls/del"},
		{RoleName: "Standard", Operation: "/job-tasks"},
		{RoleName: "Standard", Operation: "/job-tasks/add"},
		{RoleName: "Standard", Operation: "/job-tasks/put"},
		{RoleName: "Standard", Operation: "/recording-rules"},
		{RoleName: "Standard", Operation: "/recording-rules/add"},
		{RoleName: "Standard", Operation: "/recording-rules/put"},
		{RoleName: "Standard", Operation: "/recording-rules/del"},
	}

	entries := []struct {
		name  string
		entry interface{}
	}{
		{
			name:  "InitUser",
			entry: &InitPostgresUser{ID: 1, Username: "root", Nickname: "超管", Password: "root.2020", Roles: "Admin", CreateAt: time.Now().Unix(), CreateBy: "system", UpdateAt: time.Now().Unix(), UpdateBy: "system"},
		},
		{
			name:  "InitUserGroup",
			entry: &InitUserGroup{ID: 1, Name: "demo-root-group", CreateAt: time.Now().Unix(), CreateBy: "root", UpdateAt: time.Now().Unix(), UpdateBy: "root"},
		},
		{
			name:  "InitUserGroupMember",
			entry: &InitUserGroupMember{GroupID: 1, UserID: 1},
		},
		{
			name:  "InitRole",
			entry: &InitRole{Name: "Admin", Note: "Administrator role"},
		},
		{
			name:  "InitRole",
			entry: &InitRole{Name: "Standard", Note: "Ordinary user role"},
		},
		{
			name:  "InitRole",
			entry: &InitRole{Name: "Guest", Note: "Readonly user role"},
		},
		{
			name:  "InitBusiGroup",
			entry: &InitPostgresBusiGroup{ID: 1, Name: "Default Busi Group", CreateAt: time.Now().Unix(), CreateBy: "root", UpdateAt: time.Now().Unix(), UpdateBy: "root"},
		},
		{
			name:  "InitBusiGroupMember",
			entry: &InitBusiGroupMember{BusiGroupID: 1, UserGroupID: 1, PermFlag: "rw"},
		},
		{
			name:  "InitMetricView",
			entry: &InitPostgresMetricView{Name: "Host View", Cate: 0, Configs: `{"filters":[{"oper":"=","label":"__name__","value":"cpu_usage_idle"}],"dynamicLabels":[],"dimensionLabels":[{"label":"ident","value":""}]}`},
		},
		{
			name:  "InitAlertAggrView",
			entry: &InitPostgresAlertAggrView{Name: "By BusiGroup, Severity", Rule: "field:group_name::field:severity", Cate: 0},
		},
		{
			name:  "InitAlertAggrView",
			entry: &InitPostgresAlertAggrView{Name: "By RuleName", Rule: "field:rule_name", Cate: 0},
		},
	}

	for _, roleOperation := range roleOperations {
		err := db.Create(&roleOperation).Error
		if err != nil {
			logger.Errorf("[postgres database init]create role operation error: %v", err)
		}
	}

	for _, entry := range entries {
		if err := db.Create(entry.entry).Error; err != nil {
			logger.Errorf("[postgres database init]create %s error: %v", entry.name, err)
		}
	}

	return nil
}
