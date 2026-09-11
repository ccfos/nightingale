package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/strx"
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

const (
	NotiStatusSuccess = iota + 1
	NotiStatusFailure
	NotiStatusMuted // 命中「只屏蔽通知」规则，事件已产生但通知被抑制
)

// NotiChannelMuted 「只屏蔽通知」通知记录的伪渠道标识：稳定英文 key，
// 落库与 API 响应均用它，由前端按 key 翻译展示；展示层对该渠道的 Target 免脱敏。
const NotiChannelMuted = "mute"

// NotificationRecord 除 idx_evt 外还有两个索引，但都不在这里用 tag 声明：
// idx_nr_rule_created_evt(notify_rule_id, created_at, event_id) 服务「按通知规则 +
// 时间窗取通知过的事件」，event_id 放末位使其成为覆盖索引不回表；
// idx_nr_created_at(created_at) 服务每天按保留期清理的范围删除。
// 通知记录是持续高频写入的大表，用 tag 声明会让 AutoMigrate 发出通用 CREATE INDEX，
// 在 PostgreSQL 上会锁住整表写入数分钟。两个索引改由 migrate.migrateNotificationRecordIndexes
// 按数据库方言在线创建（PG 用 CONCURRENTLY、MySQL 用 LOCK=NONE），建表脚本里则直接内联。
// 索引名统一带 nr 前缀：PostgreSQL 的索引名在 schema 内唯一而非表内唯一，避免跨表撞名
type NotificationRecord struct {
	Id           int64  `json:"id" gorm:"primaryKey;type:bigint;autoIncrement"`
	NotifyRuleID int64  `json:"notify_rule_id" gorm:"type:bigint;comment:notify rule id"`
	EventId      int64  `json:"event_id" gorm:"type:bigint;not null;index:idx_evt,priority:1;comment:event history id"`
	SubId        int64  `json:"sub_id" gorm:"type:bigint;comment:subscribed rule id"`
	Channel      string `json:"channel" gorm:"type:varchar(255);not null;comment:notification channel name"`
	Status       int    `json:"status" gorm:"type:int;comment:notification status"` // 1-成功，2-失败
	Target       string `json:"target" gorm:"type:varchar(1024);not null;comment:notification target"`
	Details      string `json:"details" gorm:"type:varchar(2048);default:'';comment:notification other info"`
	CreatedAt    int64  `json:"created_at" gorm:"type:bigint;not null;comment:create time"`
}

func NewNotificationRecord(event *AlertCurEvent, notifyRuleID int64, channel, target string) *NotificationRecord {
	return &NotificationRecord{
		NotifyRuleID: notifyRuleID,
		EventId:      event.Id,
		SubId:        event.SubRuleId,
		Channel:      channel,
		Status:       NotiStatusSuccess,
		Target:       target,
	}
}

func (n *NotificationRecord) SetStatus(status int) {
	if n == nil {
		return
	}
	n.Status = status
}

func (n *NotificationRecord) SetDetails(details string) {
	if n == nil {
		return
	}
	n.Details = details
}

func (n *NotificationRecord) TableName() string {
	return "notification_record"
}

func (n *NotificationRecord) Add(ctx *ctx.Context) error {
	return Insert(ctx, n)
}

func (n *NotificationRecord) GetGroupIds(ctx *ctx.Context) (groupIds []int64) {
	if n == nil {
		return
	}

	if n.SubId > 0 {
		if sub, err := AlertSubscribeGet(ctx, "id=?", n.SubId); err != nil {
			logger.Errorf("AlertSubscribeGet failed, err: %v", err)
		} else {
			groupIds = strx.IdsInt64ForAPI(sub.UserGroupIds, " ")
		}
		return
	}

	if event, err := AlertHisEventGetById(ctx, n.EventId); err != nil {
		logger.Errorf("AlertHisEventGetById failed, err: %v", err)
	} else {
		groupIds = strx.IdsInt64ForAPI(event.NotifyGroups, " ")
	}
	return
}

func NotificationRecordsGetByEventId(ctx *ctx.Context, eid int64) ([]*NotificationRecord, error) {
	return NotificationRecordsGet(ctx, "event_id=?", eid)
}

func NotificationRecordsGet(ctx *ctx.Context, where string, args ...interface{}) ([]*NotificationRecord, error) {
	var lst []*NotificationRecord
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	return lst, nil
}

// NotifiedEventIdsScope 把事件范围限定为「指定通知规则在时间窗内实际通知过的事件」。
// 以子查询下推到 SQL 而不是先把 event_id 捞进内存再用 IN 列表拼回去：单条规则一周内
// 可能有百万级通知记录，去重后的事件 id 也可能上十万，塞进 SQL 会撑爆语句长度；
// 下推后配合 idx_nr_rule_created_evt 是索引覆盖扫描，且分页/计数都由外层查询统一负责。
// IN 子查询本身即集合判定，无需再写 DISTINCT/GROUP BY 逼出临时表
func NotifiedEventIdsScope(ctx *ctx.Context, notifyRuleID, stime, etime int64) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		sub := DB(ctx).Model(&NotificationRecord{}).Select("event_id").
			Where("notify_rule_id = ? and created_at between ? and ?", notifyRuleID, stime, etime)
		return db.Where("id in (?)", sub)
	}
}

// NotificationRecordDeleteBefore 删除一批 created_at 早于 before 的记录，返回本批实际删除行数。
// 先取主键再按主键删：DELETE ... LIMIT 只有 MySQL 支持，PostgreSQL 上会语法报错，
// 而 DELETE 里对自身表做子查询又过不了 MySQL 的限制，取 id 再删是三种数据库都成立的写法
func NotificationRecordDeleteBefore(ctx *ctx.Context, before int64, batchSize int) (int64, error) {
	// 不加 ORDER BY：删哪一批都等价，而排序会让「idx_nr_created_at 没建出来」这一
	// 本就允许发生的状态（在线 DDL 不被支持时只记日志）代价急剧放大——带排序必须把
	// 所有满足条件的行都扫一遍做 top-N，每批都是一次全表扫描 + filesort；不带排序则
	// 攒够 batchSize 行即停。有索引时优化器照样能用它做范围扫描
	var ids []int64
	err := DB(ctx).Model(&NotificationRecord{}).Where("created_at < ?", before).
		Limit(batchSize).Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}

	if len(ids) == 0 {
		return 0, nil
	}

	result := DB(ctx).Where("id in ?", ids).Delete(&NotificationRecord{})
	return result.RowsAffected, result.Error
}

// NotificationRecordLatest 取最新一条通知记录（无论成败），返回 nil 表示从未产生过通知。
// 不复用 NotificationRecordsGet：后者没有 limit，会把全部匹配行读进内存。
func NotificationRecordLatest(ctx *ctx.Context) (*NotificationRecord, error) {
	var lst []*NotificationRecord
	err := DB(ctx).Order("id desc").Limit(1).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}
