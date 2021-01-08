package models

import (
	"fmt"
	"time"

	"github.com/toolkits/pkg/cache"
	"github.com/toolkits/pkg/logger"
)

type Session struct {
	Sid         string `json:"sid"`
	AccessToken string `json:"-"`
	Username    string `json:"username"`
	RemoteAddr  string `json:"remote_addr"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
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

func (s *Session) Save() error {
	_, err := DB["rdb"].Insert(s)
	return err
}

func SessionDelete(sid string) error {
	_, err := DB["rdb"].Where("sid=?", sid).Delete(new(Session))
	return err
}

func SessionUpdate(in *Session) error {
	_, err := DB["rdb"].Where("sid=?", in.Sid).AllCols().Update(in)
	return err
}

func SessionCleanupByCreatedAt(ts int64) error {
	n, err := DB["rdb"].Where("created_at<?", ts).Delete(new(Session))
	logger.Debugf("delete before created_at %d session %d", ts, n)
	return err
}
func (s *Session) Update(cols ...string) error {
	_, err := DB["rdb"].Where("id=?", s.Sid).Cols(cols...).Update(s)
	return err
}

func SessionGetByToken(token string) (*Session, error) {
	var obj Session
	has, err := DB["rdb"].Where("access_token=?", token).Get(&obj)
	if err != nil {
		return nil, fmt.Errorf("get session err %s", err)
	}
	if !has {
		return nil, fmt.Errorf("not found")
	}

	return &obj, nil
}

// SessionGetWithCache will update session.UpdatedAt && token.LastAt
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

	if sess.Username != "" {
		cache.Set("sid."+sid, sess, time.Second*10)
	}

	return sess, nil
}

func SessionGetUserWithCache(sid string) (*User, error) {
	s, err := SessionGetWithCache(sid)
	if err != nil {
		return nil, err
	}

	if s.Username == "" {
		return nil, fmt.Errorf("user not found")
	}
	return UserMustGet("username=?", s.Username)

}
