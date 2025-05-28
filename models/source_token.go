package models

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type SourceToken struct {
	Id         int64  `json:"id" gorm:"primaryKey"`
	SourceType string `json:"source_type" gorm:"column:source_type;type:varchar(64);not null;default:''"`
	SourceId   string `json:"source_id" gorm:"column:source_id;type:varchar(255);not null;default:''"`
	Token      string `json:"token" gorm:"column:token;type:varchar(255);not null;default:''"`
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
