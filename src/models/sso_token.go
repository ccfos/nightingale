package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/toolkits/pkg/cache"
)

type Token struct {
	Id           int64  `json:"id,omitempty"`
	Name         string `json:"name,omitempty" description:"access token name"`
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ClientId     string `json:"clientId,omitempty"`
	Authorize    string `json:"authorize,omitempty"`
	Previous     string `json:"previous,omitempty"`
	ExpiresIn    int64  `json:"expiresIn,omitempty" description:"max 3 year, default:0, max time"`
	Scope        string `json:"scope,omitempty" description:"scope split by ' '"`
	RedirectUri  string `json:"redirectUri,omitempty"`
	UserName     string `json:"userName,omitempty"`
	CreatedAt    int64  `json:"createdAt,omitempty" out:",date"`
	LastAt       int64  `json:"lastAt,omitempty" out:",date"`
}

func TokenAll() (int64, error) {
	return DB["sso"].Count(new(Token))
}

func TokenGet(token string) (*Token, error) {
	var obj Token
	has, err := DB["sso"].Where("access_token=?", token).Get(&obj)
	if err != nil {
		return nil, fmt.Errorf("get token err %s", err)
	}
	if !has {
		return nil, fmt.Errorf("not found")
	}

	return &obj, nil
}

func (p *Token) Session() *Session {
	now := time.Now().Unix()
	return &Session{
		Sid:         uuid.New().String(),
		AccessToken: p.AccessToken,
		Username:    p.UserName,
		RemoteAddr:  "",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (p *Token) Update(cols ...string) error {
	_, err := DB["sso"].Where("access_token=?", p.AccessToken).Cols(cols...).Update(p)
	return err
}

func TokenDelete(token string) error {
	_, err := DB["sso"].Where("access_token=?", token).Delete(new(Token))
	return err
}

func TokenGets(where string, args ...interface{}) (tokens []Token, err error) {
	if where != "" {
		err = DB["sso"].Where(where, args...).Find(&tokens)
	} else {
		err = DB["sso"].Find(&tokens)
	}
	return
}

// TokenGetWithCache will update token.LastAt
func TokenGetWithCache(accessToken string) (*Session, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("unable to get token")
	}

	sess := &Session{}
	if err := cache.Get("access-token."+accessToken, &sess); err == nil {
		return sess, nil
	}

	var err error
	if sess, err = SessionGetByToken(accessToken); err != nil {
		// try to get token from sso
		if t, err := TokenGet(accessToken); err != nil {
			return nil, fmt.Errorf("token not found")
		} else {
			sess = t.Session()
			sess.Save()
		}
	}

	// update session
	sess.UpdatedAt = time.Now().Unix()
	sess.Update("updated_at")

	cache.Set("access-token."+accessToken, sess, time.Second*10)

	return sess, nil
}
