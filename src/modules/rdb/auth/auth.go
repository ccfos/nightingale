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

// PostLogin check user status after login
func PostLogin(user *models.User, loginErr error) error {
	return defaultAuth.PostLogin(user, loginErr)
}

func ChangePassword(user *models.User, password string) error {
	return defaultAuth.ChangePassword(user, password)
}

func CheckPassword(password string) error {
	return defaultAuth.CheckPassword(password)
}

// ChangePasswordRedirect check user should change password before login
// return change password redirect url
func ChangePasswordRedirect(user *models.User, redirect string) string {
	return defaultAuth.ChangePasswordRedirect(user, redirect)
}

func Start() error {
	return defaultAuth.Start()
}
