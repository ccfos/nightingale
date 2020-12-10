package models

import (
	"fmt"
)

type Session struct {
	Sid       string
	UserName  string
	CreatedAt int64
	UpdatedAt int64
}

func SessionAll() (int64, error) {
	return DB["rdb"].Count(new(Session))
}

func SessionUserAll(username string) (int64, error) {
	return DB["rdb"].Where("user_name=?", username).Count(new(Session))
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

	return &obj, nil
}

func SessionInsert(in *Session) error {
	_, err := DB["rdb"].Insert(in)
	return err
}

func SessionDel(sid string) error {
	_, err := DB["rdb"].Where("sid=?", sid).Delete(new(Session))
	return err
}

func SessionUpdate(in *Session) error {
	_, err := DB["rdb"].Where("sid=?", in.Sid).AllCols().Update(in)
	return err
}

func SessionCleanup(ts int64) error {
	_, err := DB["rdb"].Where("updated_at<?", ts).Delete(new(Session))
	return err
}

func (s *Session) Update(cols ...string) error {
	_, err := DB["rdb"].Where("id=?", s.Sid).Cols(cols...).Update(s)
	return err
}
