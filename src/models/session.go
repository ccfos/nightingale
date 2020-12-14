package models

import (
	"fmt"
	"time"

	"github.com/toolkits/pkg/cache"
)

type Session struct {
	Sid        string
	Username   string
	RemoteAddr string
	CreatedAt  int64
	UpdatedAt  int64
}

func SessionAll() (int64, error) {
	return DB["rdb"].Count(new(Session))
}

func SessionUserAll(username string) (int64, error) {
	return DB["rdb"].Where("username=?", username).Count(new(Session))
}

func SessionGet(sid string) (*Session, error) {
	var obj Session
	has, err := DB["rdb"].Where("sid=?", sid).Get(&obj)
	if err != nil {
		return nil, fmt.Errorf("get session err %s", err)
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

// SessionGetWithCache will update session.UpdatedAt
func SessionGetWithCache(sid string) (*Session, error) {
	if sid == "" {
		return nil, fmt.Errorf("unable to get sid")
	}

	sess := &Session{}
	if err := cache.Get("sid."+sid, &sess); err == nil {
		return sess, nil
	}

	var err error
	if sess, err = SessionGet(sid); err != nil {
		return nil, fmt.Errorf("session not found")
	}

	// update session
	sess.UpdatedAt = time.Now().Unix()
	sess.Update("updated_at")

	cache.Set("sid."+sid, sess, time.Second*30)

	return sess, nil
}

func SessionGetUserWithCache(sid string) (*User, error) {
	s, err := SessionGetWithCache(sid)
	if err != nil {
		return nil, err
	}

	return UserGet("username=?", s.Username)
}
