package models

import (
	"fmt"
	"time"
)

type LoginLog struct {
	Id       int64  `json:"id"`
	Username string `json:"username"`
	Client   string `json:"client"`
	Clock    int64  `json:"clock"`
	Loginout string `json:"loginout"`
	Err      string `json:"err"`
}

func LoginLogNew(username, client, inout string, err error) error {
	now := time.Now().Unix()
	obj := LoginLog{
		Username: username,
		Client:   client,
		Clock:    now,
		Loginout: inout,
		Err:      fmt.Sprintf("%v", err),
	}
	_, e := DB["rdb"].Insert(obj)
	return e
}

func LoginLogTotal(username string, btime, etime int64) (int64, error) {
	session := DB["rdb"].Where("clock between ? and ?", btime, etime)
	if username != "" {
		return session.Where("username like ?", username+"%").Count(new(LoginLog))
	}

	return session.Count(new(LoginLog))
}

func LoginLogGets(username string, btime, etime int64, limit, offset int) ([]LoginLog, error) {
	session := DB["rdb"].Where("clock between ? and ?", btime, etime).Limit(limit, offset).Desc("clock")
	if username != "" {
		session = session.Where("username like ?", username+"%")
	}

	var objs []LoginLog
	err := session.Find(&objs)
	return objs, err
}
