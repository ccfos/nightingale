package models

import (
	"fmt"
	"os"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type UserToken struct {
	UserId   int64  `json:"user_id"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

func (UserToken) TableName() string {
	return "user_token"
}

func UserTokenGet(where string, args ...interface{}) (*UserToken, error) {
	var obj UserToken
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query user_token fail: %v", err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func UserTokenGets(where string, args ...interface{}) ([]UserToken, error) {
	var objs []UserToken
	err := DB.Where(where, args...).OrderBy("token").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: list user_token fail: %v", err)
		return objs, internalServerError
	}

	if objs == nil {
		return []UserToken{}, nil
	}

	return objs, nil
}

func UserTokenNew(userId int64, username string) (*UserToken, error) {
	items, err := UserTokenGets("user_id=?", userId)
	if err != nil {
		return nil, err
	}

	if len(items) >= 2 {
		return nil, _e("Each user has at most two tokens")
	}

	obj := UserToken{
		UserId:   userId,
		Username: username,
		Token:    genToken(userId),
	}

	err = DBInsertOne(obj)
	if err != nil {
		return nil, err
	}

	return &obj, nil
}

func UserTokenReset(userId int64, token string) (*UserToken, error) {
	obj, err := UserTokenGet("token=? and user_id=?", token, userId)
	if err != nil {
		return nil, err
	}

	if obj == nil {
		return nil, _e("No such token")
	}

	obj.Token = genToken(userId)
	_, err = DB.Where("user_id=? and token=?", userId, token).Cols("token").Update(obj)
	if err != nil {
		logger.Errorf("mysql.error: update user_token fail: %v", err)
		return nil, internalServerError
	}

	return obj, nil
}

func genToken(userId int64) string {
	now := time.Now().UnixNano()
	rls := str.RandLetters(6)
	return str.MD5(fmt.Sprintf("%d%d%d%s", os.Getpid(), userId, now, rls))
}
