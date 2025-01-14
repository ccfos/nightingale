package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"sync"
	"time"
)

type Token struct {
	Id        int64  `json:"id" gorm:"primaryKey"`
	Username  string `json:"username"`
	TokenName string `json:"token_name" gorm:"type:varchar(255)"`
	Token     string `json:"token" gorm:"type:varchar(255);uniqueIndex"`
}

func (Token) TableName() string {
	return "token"
}

var TokenToUser sync.Map

func init() {
	// 定时清空缓存
	tick := time.Tick(1 * time.Minute)
	go func() {
		for {
			select {
			case <-tick:
				TokenToUser = sync.Map{}
			}
		}
	}()
}

func CountToken(ctx *ctx.Context, username string) (int64, error) {
	var count int64
	err := DB(ctx).Model(&Token{}).Where("username = ?", username).Count(&count).Error
	return count, err
}

func AddToken(ctx *ctx.Context, username, token, tokenName string) (*Token, error) {
	newToken := Token{
		TokenName: tokenName,
		Username:  username,
		Token:     token,
	}

	err := Insert(ctx, &newToken)
	return &newToken, err
}

func DeleteToken(ctx *ctx.Context, token string) error {
	err := DB(ctx).Where("token = ?", token).Delete(&Token{}).Error
	if err == nil {
		TokenToUser.Delete(token)
	}
	return err
}

func GetTokensByUsername(ctx *ctx.Context, username string) ([]Token, error) {
	var tokens []Token
	err := DB(ctx).Where("username = ?", username).Find(&tokens).Error
	return tokens, err
}

func GetUserByToken(ctx *ctx.Context, token string) (*User, error) {
	val, hit := TokenToUser.Load(token)
	if user, ok := val.(*User); hit && ok {
		return user, nil
	}
	var user User
	err := DB(ctx).Select("users.id, users.username").
		Joins("JOIN token t ON t.username = users.username").
		Where("t.token = ?", token).First(&user).Debug().Error
	if user.Username != "" {
		TokenToUser.Store(token, &user)
	}
	return &user, err
}
