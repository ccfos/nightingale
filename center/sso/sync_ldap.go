package sso

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

const LDAPNAME = "ldap"

func (s *SsoClient) SyncSsoUsers(ctx *ctx.Context) {
	err := s.syncSsoUsers(ctx)
	if err != nil {
		fmt.Println("failed to sync ldap:", err)
	}

	go s.loopSyncSsoUsers(ctx)
}

func (s *SsoClient) loopSyncSsoUsers(ctx *ctx.Context) {
	var err error
	for {
		select {
		case <-s.LDAP.Ticker.C:
			if s.LDAP.SyncUsers {
				err = s.syncSsoUsers(ctx)
			} else {
				err = s.syncDelSsoUser(ctx)
			}

			if err != nil {
				logger.Warningf("failed to sync ldap: %v", err)
			}
		}
	}
}

func (s *SsoClient) syncSsoUsers(ctx *ctx.Context) error {
	if !s.LDAP.Enable || !s.LDAP.SyncUsers {
		return nil
	}

	start := time.Now()

	usersFromSso, usersFromDb, err := s.getUsersFromSsoAndDb(ctx)
	if err != nil {
		dumper.PutSyncRecord("sso_user", start.Unix(), -1, -1, "failed to query users: "+err.Error())
		return err
	}

	usersToBeDel := getExtraUsers(usersFromSso, usersFromDb)
	if len(usersToBeDel) > 0 {
		delIds := make([]int64, 0, len(usersToBeDel))
		for _, user := range usersToBeDel {
			delIds = append(delIds, user.Id)
		}
		if err := models.UserDelByIds(ctx, delIds); err != nil {
			logger.Warningf("failed to sync del users[%v] to db, err: %v", usersToBeDel, err)
		}
	}

	usersToBeAdd := getExtraUsers(usersFromDb, usersFromSso)
	for _, user := range usersToBeAdd {
		if err := user.Add(ctx); err != nil {
			logger.Warningf("failed to sync add user[%v] to db, err: %v", *user, err)
		}
	}

	ms := time.Since(start).Milliseconds()
	logger.Infof("timer: sync sso users done, cost: %dms, number: %d", ms, len(usersToBeDel)+len(usersToBeAdd))
	dumper.PutSyncRecord("sso_user", start.Unix(), ms, len(usersToBeDel)+len(usersToBeAdd), "success")
	return nil
}

func (s *SsoClient) syncDelSsoUser(ctx *ctx.Context) error {
	start := time.Now()

	usersFromDb, err := models.UserGetsBySso(ctx, LDAPNAME)
	if err != nil {
		dumper.PutSyncRecord("sso_user", start.Unix(), -1, -1, "failed to query users: "+err.Error())
		return err
	}

	delIds := make([]int64, 0)
	for _, user := range usersFromDb {
		exist, err := s.LDAP.UserExist(user.Username)
		if err != nil {
			dumper.PutSyncRecord("sso_user", start.Unix(), -1, -1, "failed to check whether the user exists: "+err.Error())
		} else if !exist {
			delIds = append(delIds, user.Id)
		}
	}

	if len(delIds) > 0 {
		if err := models.UserDelByIds(ctx, delIds); err != nil {
			dumper.PutSyncRecord("sso_user", start.Unix(), -1, -1, "failed to sync del users: "+err.Error())
			return err
		}
	}

	dumper.PutSyncRecord("sso_user", start.Unix(), time.Since(start).Milliseconds(), len(delIds), "success")
	return nil
}

func (s *SsoClient) getUsersFromSsoAndDb(ctx *ctx.Context) (usersFromSso, usersFromDb map[string]*models.User, err error) {
	usersFromSso, err = s.LDAP.UserGetAll()
	if err != nil {
		return nil, nil, err
	}

	usersFromDb, err = models.UserGetsBySso(ctx, LDAPNAME)
	if err != nil {
		return nil, nil, err
	}

	return
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
