package models

import (
	"errors"
	"time"
)

type Captcha struct {
	CaptchaId string `json:"captchaId"`
	Answer    string `json:"-"`
	Image     string `xorm:"-" json:"image"`
	CreatedAt int64  `json:"createdAt"`
}

func CaptchaGet(where string, args ...interface{}) (*Captcha, error) {
	var obj Captcha
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, errors.New("captcha not found")
	}

	return &obj, nil
}

func (p *Captcha) Save() error {
	_, err := DB["rdb"].Insert(p)
	return err
}

func (p *Captcha) Del() error {
	_, err := DB["rdb"].Where("captcha_id=?", p.CaptchaId).Delete(new(Captcha))
	return err
}

const captchaExpiresIn = 600

func (p Captcha) CleanUp() error {
	_, err := DB["rdb"].Exec("delete from captcha where created_at < ?", time.Now().Unix()-captchaExpiresIn)
	return err
}
