package cron

import (
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/toolkits/pkg/logger"
)

func UserManagerLoop() {
	cf := config.Config.Auth
	for {
		now := time.Now().Unix()
		if cf.UserExpire {
			// 3个月以上未登录，用户自动变为休眠状态
			if _, err := models.DB["rdb"].Exec("update user set status=?, updated_at=?, locked_at=? where ((logged_at > 0 and logged_at<?) or (logged_at == 0 and created_at < ?)) and status in (?,?,?)",
				models.USER_S_FROZEN, now, now, now-90*86400,
				models.USER_S_ACTIVE, models.USER_S_INACTIVE, models.USER_S_LOCKED); err != nil {
				logger.Errorf("update user status error %s", err)
			}

			// 变为休眠状态后1年未激活，用户自动变为已注销状态
			if _, err := models.DB["rdb"].Exec("update user set status=?, updated_at=? where locked_at<? and status=?",
				models.USER_S_WRITEN_OFF, now, now-365*86400, models.USER_S_FROZEN); err != nil {
				logger.Errorf("update user status error %s", err)
			}
		}

		// reset login err num before 24 hours ago
		if _, err := models.DB["rdb"].Exec("update user set login_err_num=0, updated_at=? where updated_at<? and login_err_num>0", now, now-86400); err != nil {
			logger.Errorf("update user login err num error %s", err)
		}

		time.Sleep(time.Hour)
	}
}
