package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"sync"
	"time"
)

type UserToken struct {
	Id        int64  `json:"id" gorm:"primaryKey"`
	Username  string `json:"username" gorm:"type:varchar(255)"`
	TokenName string `json:"token_name" gorm:"type:varchar(255)"`
	Token     string `json:"token" gorm:"type:varchar(255);uniqueIndex"`
}

func (UserToken) TableName() string {
	return "user_token"
}

var TokenToUser map[string]*User
var lock sync.RWMutex

func SyncTokenToUser(ctx *ctx.Context) {
	syncTokenToUser(ctx)
	// 定时同步 token
	tick := time.Tick(1 * time.Minute)
	go func() {
		for {
			select {
			case <-tick:
				syncTokenToUser(ctx)
			}
		}
	}()
}

func syncTokenToUser(ctx *ctx.Context) {
	var tokens []struct {
		Id       int64  `json:"id" gorm:"id"`
		Username string `json:"username" gorm:"username"`
		Token    string `json:"token" gorm:"token"`
	}
	err := DB(ctx).Table("users").
		Select("users.id, users.username, t.token").
		Joins("JOIN user_token t ON t.username = users.username").
		Scan(&tokens).Error
	if err != nil {
		return
	}
	tokenToUser := make(map[string]*User)
	for _, token := range tokens {
		tokenToUser[token.Token] = &User{
			Id:       token.Id,
			Username: token.Username,
		}
	}
	TokenToUser = tokenToUser
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
	}

	err := Insert(ctx, &newToken)
	if err == nil {
		lock.Lock()
		TokenToUser[token] = &User{
			Username: username,
		}
		lock.Unlock()
	}
	return &newToken, err
}

func DeleteToken(ctx *ctx.Context, token string) error {
	err := DB(ctx).Where("token = ?", token).Delete(&UserToken{}).Error
	if err == nil {
		lock.Lock()
		delete(TokenToUser, token)
		lock.Unlock()
	}
	return err
}

func GetTokensByUsername(ctx *ctx.Context, username string) ([]UserToken, error) {
	var tokens []UserToken
	err := DB(ctx).Where("username = ?", username).Find(&tokens).Error
	return tokens, err
}

func GetUserByToken(ctx *ctx.Context, token string) *User {
	lock.RLock()
	defer lock.RUnlock()
	return TokenToUser[token]
}
