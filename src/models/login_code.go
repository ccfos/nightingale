package models

type LoginCode struct {
	Username  string `json:"username"`
	Code      string `json:"code"`
	LoginType string `json:"login_type"`
	CreatedAt int64  `json:"created_at"`
}

func LoginCodeGet(where string, args ...interface{}) (*LoginCode, error) {
	var obj LoginCode
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (p *LoginCode) Save() error {
	p.Del()
	_, err := DB["rdb"].Insert(p)
	return err
}

func (p *LoginCode) Del() error {
	_, err := DB["rdb"].Where("username=?", p.Username).Delete(new(LoginCode))
	return err
}
