package auth

import (
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
)

var defaultAuth Authenticator

func Init(cf config.AuthExtraSection) {
	defaultAuth = *New(cf)
}

func WhiteListAccess(remoteAddr string) error {
	return defaultAuth.WhiteListAccess(remoteAddr)
}

func PostLogin(user *models.User, loginErr error) error {
	return defaultAuth.PostLogin(user, loginErr)
}

func ChangePassword(user *models.User, password string) error {
	return defaultAuth.ChangePassword(user, password)
}

func CheckPassword(password string) error {
	return defaultAuth.CheckPassword(password)
}

func Start() error {
	return defaultAuth.Start()
}
