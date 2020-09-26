package models

import (
	"time"
)

type Invite struct {
	Id      int64
	Token   string
	Expire  int64
	Creator string
}

func InviteGet(where string, args ...interface{}) (*Invite, error) {
	var obj Invite
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func InviteNew(token, creator string) error {
	now := time.Now().Unix()
	obj := Invite{
		Token:   token,
		Creator: creator,
		Expire:  now + 3600*24,
	}
	_, err := DB["rdb"].Insert(obj)
	return err
}
