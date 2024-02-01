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

const LDAPNAME = "ldap"

func (s *SsoClient) SyncSsoUsers(ctx *ctx.Context) {
	err := s.syncSsoUsers(ctx)
	if err != nil {
		fmt.Println("failed to sync LDAPNAME:", err)
	}

	go s.loopSyncSsoUsers(ctx)
}

func (s *SsoClient) loopSyncSsoUsers(ctx *ctx.Context) {
	duration := s.LDAP.SyncCycle
	if duration == 0 {
		duration = 24 * time.Hour
	}
	for {
		time.Sleep(duration)
		if err := s.syncSsoUsers(ctx); err != nil {
			logger.Warningf("failed to sync %s: %v", LDAPNAME, err)
		}
	}
}

func (s *SsoClient) syncSsoUsers(ctx *ctx.Context) error {
	if !s.LDAP.Enable {
		return nil
	}

	start := time.Now()

	usersFromSso, err := s.LDAP.LdapGetAllUsers()
	if err != nil {
		dumper.PutSyncRecord("sso_users", start.Unix(), -1, -1, "failed to query all users: "+err.Error())
		return errors.WithMessage(err, "failed to exec LdapGetAllUsers")
	}

	usersFromDb, err := models.UserGetsBySso(ctx, LDAPNAME)
	if err != nil {
		return err
	}

	usersToBeDel := getExtraUsers(usersFromSso, usersFromDb)
	if len(usersToBeDel) > 0 {
		delIds := make([]int64, 0, len(usersToBeDel))
		for _, user := range usersToBeDel {
			delIds = append(delIds, user.Id)
		}
		if err := models.UsersDelByIds(ctx, delIds); err != nil {
			logger.Warningf("failed to sync del users[%v] to db,err: %v", usersToBeDel, err)
		}
	}

	if !s.LDAP.SyncUsers {
		return nil
	}

	usersToBeAdd := getExtraUsers(usersFromDb, usersFromSso)
	failedNum := 0
	for _, user := range usersToBeAdd {
		user.Belong = LDAPNAME
		if err := user.Add(ctx); err != nil {
			logger.Warningf("failed to sync user[%v] to db, err: %v", *user, err)
			failedNum++
		}
	}

	ms := time.Since(start).Milliseconds()
	logger.Infof("timer: sync sso users done, cost: %dms, number: %d, number: %d", ms, len(usersToBeDel)+len(usersToBeAdd))
	dumper.PutSyncRecord("sso_user", start.Unix(), ms, len(usersToBeDel)+len(usersToBeAdd), "success")
	return nil
}

// in m not in base
func getExtraUsers(base, m map[string]*models.User) (extraUsers []*models.User) {
	for username, user := range m {
		if _, exist := base[username]; !exist {
			extraUsers = append(extraUsers, user)
		}
	}
	return
}
