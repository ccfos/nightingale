package sso

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

const ldap = "ldap"

func (s *SsoClient) SyncSsoUsers(ctx *ctx.Context) {
	err := s.syncSsoUsers(ctx)
	if err != nil {
		fmt.Println("failed to sync ldap:", err)
	}

	go s.loopSyncSsoUsers(ctx)
}

func (s *SsoClient) loopSyncSsoUsers(ctx *ctx.Context) {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := s.syncSsoUsers(ctx); err != nil {
			logger.Warning("failed to sync ldap:", err)
		}
	}
}

func (s *SsoClient) syncSsoUsers(ctx *ctx.Context) error {
	start := time.Now()
	if !s.LDAP.SyncToMysql {
		return nil
	}

	usersSso, err := s.LDAP.LdapGetAllUsers()
	if err != nil {
		dumper.PutSyncRecord("sso_users", start.Unix(), -1, -1, "failed to query all users: "+err.Error())
		return errors.WithMessage(err, "failed to exec LdapGetAllUsers")
	}

	usersDbArr, err := models.UserGetsBySso(ctx, ldap)
	usersDbMap := make(map[string]*models.User, len(usersDbArr))
	for i, user := range usersDbArr {
		usersDbMap[user.Username] = &usersDbArr[i]
	}

	del, add, update := diff(usersDbMap, usersSso)

	if len(del) > 0 {
		delIds := make([]int64, 0, len(del))
		for _, user := range del {
			delIds = append(delIds, user.Id)
		}
		if err := models.SsoUsersDelByIds(ctx, delIds, ldap); err != nil {
			return err
		}
	}

	for _, user := range add {
		if err := user.AddSso(ctx, ldap); err != nil {
			return err
		}
	}

	for _, user := range update {
		user.UpdateAt = time.Now().Unix()
		if err := user.UpdateSso(ctx, ldap, "email", "nickname", "phone", "update_at"); err != nil {
			return err
		}
	}

	ms := time.Since(start).Milliseconds()
	logger.Infof("timer: sync sso users done, cost: %dms, number: %d", ms, len(del)+len(add)+len(update))
	dumper.PutSyncRecord("sso_user", start.Unix(), ms, len(del)+len(add)+len(update), "success")

	return nil
}

func diff(base, m map[string]*models.User) (del, add, update []*models.User) {
	for username, user := range m {
		baseUser, exist := base[username]
		if !exist {
			add = append(add, user)
		} else {
			if baseUser.Nickname != user.Nickname || baseUser.Phone != user.Phone || baseUser.Email != user.Email {
				update = append(update, user)
			}
		}
	}
	// if user in base but not in m, delete it
	for username, user := range base {
		if _, exist := m[username]; !exist {
			del = append(del, user)
		}
	}

	return
}
