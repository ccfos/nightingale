package models

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type UserToken struct {
	Id        int64  `json:"id" gorm:"primaryKey"`
	Username  string `json:"username" gorm:"type:varchar(255); not null; default ''"`
	TokenName string `json:"token_name" gorm:"type:varchar(255); not null; default ''"`
	Token     string `json:"token" gorm:"type:varchar(255); not null; default ''"`
	CreateAt  int64  `json:"create_at" gorm:"type:bigint; not null; default 0"`
	LastUsed  int64  `json:"last_used" gorm:"type:bigint; not null; default 0"`
}

func (UserToken) TableName() string {
	return "user_token"
}

func CountToken(ctx *ctx.Context, username string) (int64, error) {
	var count int64
	err := DB(ctx).Model(&UserToken{}).Where("username = ?", username).Count(&count).Error
	return count, err
}

func AddToken(ctx *ctx.Context, username, token, tokenName string) (*UserToken, error) {
	newToken := UserToken{
		TokenName: tokenName,
		Username:  username,
		Token:     token,
		CreateAt:  time.Now().Unix(),
	}

	err := Insert(ctx, &newToken)
	return &newToken, err
}

// DeleteToken deletes the token only if it belongs to username and reports
// whether a row was deleted.
func DeleteToken(ctx *ctx.Context, id int64, username string) (bool, error) {
	res := DB(ctx).Where("id = ? and username = ?", id, username).Delete(&UserToken{})
	return res.RowsAffected > 0, res.Error
}

func GetTokensByUsername(ctx *ctx.Context, username string) ([]UserToken, error) {
	var tokens []UserToken
	err := DB(ctx).Where("username = ?", username).Find(&tokens).Error
	return tokens, err
}

func UserTokenGetAll(ctx *ctx.Context) ([]*UserToken, error) {
	var lst []*UserToken
	err := DB(ctx).Find(&lst).Error
	return lst, err
}

// UserTokenStatistics returns the row count and max(id). Token rows never change
// id/username/token after insert and ids are never reused, so any create or
// delete between two calls changes at least one of them.
func UserTokenStatistics(ctx *ctx.Context) (total, maxId int64, err error) {
	var stat struct {
		Total int64
		MaxId int64
	}
	err = DB(ctx).Model(&UserToken{}).
		Select("count(*) as total, coalesce(max(id), 0) as max_id").
		Scan(&stat).Error
	return stat.Total, stat.MaxId, err
}

func UserTokenUpdateLastUsedTime(ctx *ctx.Context, token string, lastUsedTime int64) error {
	return DB(ctx).Model(&UserToken{}).Where("token = ?", token).Update("last_used", lastUsedTime).Error
}
