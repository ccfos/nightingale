package models

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

const (
	SourceTypeEvent = "event"
	SourceTypeBoard = "board"
)

type SourceToken struct {
	Id         int64  `json:"id" gorm:"primaryKey"`
	SourceType string `json:"source_type" gorm:"column:source_type;type:varchar(64);not null;default:''"`
	SourceId   string `json:"source_id" gorm:"column:source_id;type:varchar(255);not null;default:''"`
	Token      string `json:"token" gorm:"column:token;type:varchar(255);not null;default:'';index:idx_source_token_token"`
	Note       string `json:"note" gorm:"type:varchar(255);not null;default:''"`
	ExpireAt   int64  `json:"expire_at" gorm:"type:bigint;not null;default:0"`
	CreateAt   int64  `json:"create_at" gorm:"type:bigint;not null;default:0"`
	CreateBy   string `json:"create_by" gorm:"type:varchar(64);not null;default:''"`
}

func (SourceToken) TableName() string {
	return "source_token"
}

func (st *SourceToken) Add(ctx *ctx.Context) error {
	return Insert(ctx, st)
}

// GetSourceTokenBySource 根据源类型和源ID获取源令牌
func GetSourceTokenBySource(ctx *ctx.Context, sourceType, sourceId, token string) (*SourceToken, error) {
	var st SourceToken
	err := DB(ctx).Where("source_type = ? AND source_id = ? AND token = ?", sourceType, sourceId, token).First(&st).Error
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// GetSourceTokenByToken 按令牌反查记录，调用方无需预知 source_id，
// 用于携带 __token 的数据查询请求定位其绑定的资源。未找到时返回 (nil, nil)
func GetSourceTokenByToken(ctx *ctx.Context, sourceType, token string) (*SourceToken, error) {
	if token == "" {
		return nil, nil
	}

	var lst []*SourceToken
	err := DB(ctx).Where("source_type = ? AND token = ?", sourceType, token).Limit(1).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

// SourceTokenGets 列出某个资源已签发的令牌，按创建时间倒序。
// 含已过期记录：过期项也要让使用者看到并可清理
func SourceTokenGets(ctx *ctx.Context, sourceType, sourceId string) ([]*SourceToken, error) {
	var lst []*SourceToken
	err := DB(ctx).Where("source_type = ? AND source_id = ?", sourceType, sourceId).
		Order("create_at desc").Find(&lst).Error
	if err != nil {
		return nil, err
	}
	return lst, nil
}

func SourceTokenGetById(ctx *ctx.Context, id int64) (*SourceToken, error) {
	var lst []*SourceToken
	err := DB(ctx).Where("id = ?", id).Limit(1).Find(&lst).Error
	if err != nil {
		return nil, err
	}
	if len(lst) == 0 {
		return nil, nil
	}
	return lst[0], nil
}

// SourceTokenDel 注销令牌：删除后该分享链接立即失效
func SourceTokenDel(ctx *ctx.Context, id int64) error {
	return DB(ctx).Where("id = ?", id).Delete(&SourceToken{}).Error
}

func (st *SourceToken) IsExpired() bool {
	if st.ExpireAt == 0 {
		return false // 0 表示永不过期
	}
	return time.Now().Unix() > st.ExpireAt
}

func CleanupExpiredTokens(ctx *ctx.Context) (int64, error) {
	now := time.Now().Unix()
	result := DB(ctx).Where("expire_at > 0 AND expire_at < ?", now).Delete(&SourceToken{})
	return result.RowsAffected, result.Error
}
