package models

import (
	"errors"
	"time"
)

type AuthState struct {
	State     string `json:"state"`
	Typ       string `json:"typ"`
	Redirect  string `json:"redirect"`
	ExpiresAt int64  `json:"expiresAt"`
}

func AuthStateGet(where string, args ...interface{}) (*AuthState, error) {
	var obj AuthState
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, errors.New("auth state not found")
	}

	return &obj, nil
}

func (p *AuthState) Save() error {
	_, err := DB["rdb"].Insert(p)
	return err
}

func (p *AuthState) Del() error {
	_, err := DB["rdb"].Where("state=?", p.State).Delete(new(AuthState))
	return err
}

func (p AuthState) CleanUp() error {
	_, err := DB["rdb"].Exec("delete from auth_state where expires_at < ?", time.Now().Unix())
	return err
}
