package models

import (
	"fmt"
	"os"
	"time"

	"github.com/toolkits/pkg/str"
)

type UserToken struct {
	UserId   int64  `json:"user_id"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

func UserTokenGet(where string, args ...interface{}) (*UserToken, error) {
	var obj UserToken
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func UserTokenGets(where string, args ...interface{}) ([]UserToken, error) {
	var objs []UserToken
	err := DB["rdb"].Where(where, args...).Find(&objs)
	return objs, err
}

func UserTokenNew(userId int64, username string) (*UserToken, error) {
	items, err := UserTokenGets("user_id=?", userId)
	if err != nil {
		return nil, err
	}

	if len(items) >= 2 {
		return nil, fmt.Errorf("each user has at most two tokens")
	}

	obj := UserToken{
		UserId:   userId,
		Username: username,
		Token:    genToken(userId),
	}

	_, err = DB["rdb"].Insert(obj)
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
		return nil, fmt.Errorf("no such token")
	}

	obj.Token = genToken(userId)
	_, err = DB["rdb"].Where("user_id=? and token=?", userId, token).Cols("token").Update(obj)
	return obj, err
}

func genToken(userId int64) string {
	now := time.Now().UnixNano()
	rls := str.RandLetters(6)
	return str.MD5(fmt.Sprintf("%d%d%d%s", os.Getpid(), userId, now, rls))
}
