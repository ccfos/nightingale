package models

import (
	"encoding/json"
	"fmt"
)

type Session struct {
	Sid        string
	Data_      string            `xorm:"data"`
	Data       map[string]string `xorm:"-"`
	UserName   string
	CookieName string
	CreatedAt  int64
	UpdatedAt  int64
}

func SessionAll(cookieName string) (int64, error) {
	return DB["rdb"].Where("cookie_name=?", cookieName).Count(new(Session))
}

func SessionUserAll(cookieName, username string) (int64, error) {
	return DB["rdb"].Where("cookie_name=? and user_name=?", cookieName, username).Count(new(Session))
}

func SessionGet(sid string) (*Session, error) {
	var obj Session
	has, err := DB["rdb"].Where("sid=?", sid).Get(&obj)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, fmt.Errorf("not found")
	}

	err = json.Unmarshal([]byte(obj.Data_), &obj.Data)
	if err != nil {
		return nil, err
	}

	return &obj, nil
}

func SessionInsert(in *Session) error {
	b, err := json.Marshal(in.Data)
	if err != nil {
		return err
	}
	in.Data_ = string(b)

	_, err = DB["rdb"].Insert(in)
	return err
}

func SessionDel(sid string) error {
	_, err := DB["rdb"].Where("sid=?", sid).Delete(new(Session))
	return err
}

func SessionUpdate(in *Session) error {
	b, err := json.Marshal(in.Data)
	if err != nil {
		return err
	}
	in.Data_ = string(b)

	_, err = DB["rdb"].Where("sid=?", in.Sid).AllCols().Update(in)
	return err
}

func SessionCleanup(ts int64, cookieName string) error {
	_, err := DB["rdb"].Where("updated_at<? and cookie_name=?", ts, cookieName).Delete(new(Session))
	return err
}

// unsafe for in.Data
func (s *Session) Update(cols ...string) error {
	_, err := DB["rdb"].Where("id=?", s.Sid).Cols(cols...).Update(s)
	return err
}
