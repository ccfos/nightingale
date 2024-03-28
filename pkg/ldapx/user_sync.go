package ldapx

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

func (s *SsoClient) SyncAddAndDelUsers(ctx *ctx.Context) error {
	if !s.Enable || !s.SyncAdd {
		return nil
	}

	start := time.Now()

	usersFromSso, usersFromDb, err := s.getUsersFromSsoAndDb(ctx)
	if err != nil {
		dumper.PutSyncRecord("sso_user", start.Unix(), -1, -1, "failed to query users: "+err.Error())
		return err
	}

	usersToBeAdd := getExtraUsers(usersFromDb, usersFromSso)
	for _, user := range usersToBeAdd {
		if err := user.Add(ctx); err != nil {
			logger.Warningf("failed to sync add user[%v] to db, err: %v", *user, err)
		}
	}

	var usersToBeDel []*models.User
	if s.SyncDel {
		usersToBeDel = getExtraUsers(usersFromSso, usersFromDb)
		if len(usersToBeDel) > 0 {
			delIds := make([]int64, 0, len(usersToBeDel))
			for _, user := range usersToBeDel {
				delIds = append(delIds, user.Id)
			}
			if err := models.UserDelByIds(ctx, delIds); err != nil {
				logger.Warningf("failed to sync del users[%v] to db, err: %v", usersToBeDel, err)
			}
		}
	}

	ms := time.Since(start).Milliseconds()
	logger.Infof("timer: sync sso users done, cost: %dms, number: %d", ms, len(usersToBeDel)+len(usersToBeAdd))
	dumper.PutSyncRecord("sso_user", start.Unix(), ms, len(usersToBeDel)+len(usersToBeAdd), "success")
	return nil
}

func (s *SsoClient) getUsersFromSsoAndDb(ctx *ctx.Context) (usersFromSso, usersFromDb map[string]*models.User, err error) {
	usersFromSso, err = s.UserGetAll()
	if err != nil {
		return nil, nil, err
	}

	usersFromDb, err = models.UserGetsBySso(ctx, "ldap")
	if err != nil {
		return nil, nil, err
	}

	return
}

func (s *SsoClient) UserGetAll() (map[string]*models.User, error) {
	lc := s.Copy()

	conn, err := lc.newLdapConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	sr, err := lc.ldapReq(conn, lc.UserFilter)
	if err != nil {
		return nil, fmt.Errorf("ldap.error: ldap search fail: %v", err)
	}

	res := make(map[string]*models.User, len(sr.Entries))
	for _, entry := range sr.Entries {
		attrs := lc.Attributes
		username := entry.GetAttributeValue(attrs.Username)
		nickname := entry.GetAttributeValue(attrs.Nickname)
		email := entry.GetAttributeValue(attrs.Email)
		phone := entry.GetAttributeValue(attrs.Phone)

		user := new(models.User)
		user.FullSsoFields("ldap", username, nickname, phone, email, lc.DefaultRoles)

		res[entry.GetAttributeValue(attrs.Username)] = user
	}

	return res, nil
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

func (s *SsoClient) SyncDelUsers(ctx *ctx.Context) error {
	if !s.Enable || s.SyncAdd || !s.SyncDel {
		return nil
	}

	start := time.Now()

	usersFromDb, err := models.UserGetsBySso(ctx, "ldap")
	if err != nil {
		dumper.PutSyncRecord("sso_user", start.Unix(), -1, -1, "failed to query users: "+err.Error())
		return err
	}

	delIds := make([]int64, 0)
	for _, user := range usersFromDb {
		exist, err := s.UserExist(user.Username)
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

	ms := time.Since(start).Milliseconds()
	logger.Infof("timer: sync del sso users done, cost: %dms, number: %d", ms, len(delIds))
	dumper.PutSyncRecord("sso_user", start.Unix(), ms, len(delIds), "success")
	return nil
}

func (s *SsoClient) UserExist(username string) (bool, error) {
	lc := s.Copy()

	conn, err := lc.newLdapConn()
	if err != nil {
		return false, err
	}
	defer conn.Close()

	sr, err := lc.ldapReq(conn, "(&(%s=%s))", lc.Attributes.Username, username)
	if err != nil {
		return false, err
	}

	if len(sr.Entries) > 0 {
		return true, nil
	}

	return false, nil
}
