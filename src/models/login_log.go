package models

import "time"

type LoginLog struct {
	Id       int64  `json:"id"`
	Username string `json:"username"`
	Client   string `json:"client"`
	Clock    int64  `json:"clock"`
	Loginout string `json:"loginout"`
}

func LoginLogNew(username, client, inout string) error {
	now := time.Now().Unix()
	obj := LoginLog{
		Username: username,
		Client:   client,
		Clock:    now,
		Loginout: inout,
	}
	_, err := DB["rdb"].Insert(obj)
	return err
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
