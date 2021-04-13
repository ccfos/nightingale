package auth

import (
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/config"
	"github.com/didi/nightingale/v4/src/modules/server/ssoc"
)

var defaultAuth Authenticator

func Init(cf config.AuthExtraSection) {
	defaultAuth = *New(cf)
}

func WhiteListAccess(user *models.User, remoteAddr string) error {
	return defaultAuth.WhiteListAccess(user, remoteAddr)
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

func PostCallback(in *ssoc.CallbackOutput) error {
	return defaultAuth.PostCallback(in)
}

func LogoutByUsername(username string) error {
	return defaultAuth.LogoutByUsername(username)
}

func DeleteSession(sid string) error {
	return defaultAuth.DeleteSession(sid)
}

func DeleteToken(accessToken string) error {
	return defaultAuth.DeleteToken(accessToken)
}

func Start() error {
	return defaultAuth.Start()
}

func PrepareUser(user *models.User) {
	defaultAuth.PrepareUser(user)
}
