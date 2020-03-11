package model

import (
	"time"
)

type Invite struct {
	Id      int64
	Token   string
	Expire  int64
	Creator string
}

func InviteGet(col string, val interface{}) (*Invite, error) {
	var obj Invite
	has, err := DB["uic"].Where(col+"=?", val).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func InviteAdd(token, creator string) error {
	now := time.Now().Unix()
	obj := Invite{
		Token:   token,
		Creator: creator,
		Expire:  now + 3600*24*30,
	}
	_, err := DB["uic"].Insert(&obj)
	return err
}
