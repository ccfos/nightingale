package tools

import (
	"time"

	"github.com/didi/nightingale/src/models"

	"github.com/toolkits/pkg/cache"
	"github.com/toolkits/pkg/logger"
)

func UsernameByUUID(uuid string) string {
	if uuid == "" {
		return ""
	}

	var username string
	if err := cache.Get("uuid."+uuid, &username); err == nil {
		return username
	}

	value := models.UsernameByUUID(uuid)

	if value != "" {
		cache.Set("uuid."+uuid, value, time.Hour)
	} else {
		logger.Warningf("cannot get username by uuid:%v", uuid)
	}

	return value
}

func UserByUUID(uuid string) *models.User {
	user, err := models.UserGet("uuid=?", uuid)
	if err != nil {
		logger.Warningf("cannot get username by uuid:%v err:%v", uuid, err)
	}
	return user
}
