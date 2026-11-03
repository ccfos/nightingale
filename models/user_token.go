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

func DeleteToken(ctx *ctx.Context, id int64) error {
	err := DB(ctx).Where("id = ?", id).Delete(&UserToken{}).Error
	return err
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

// GetUserToken 只返回属于 username 的 token，不存在或不属于该用户时返回 nil。
func GetUserToken(ctx *ctx.Context, id int64, username string) (*UserToken, error) {
	var lst []*UserToken
	err := DB(ctx).Where("id = ? and username = ?", id, username).Find(&lst).Error
	if err != nil || len(lst) == 0 {
		return nil, err
	}
	return lst[0], nil
}

func UserTokenUpdateLastUsedTime(ctx *ctx.Context, token string, lastUsedTime int64) error {
	return DB(ctx).Model(&UserToken{}).Where("token = ?", token).Update("last_used", lastUsedTime).Error
}
