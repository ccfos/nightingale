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

	usersToBeAdd, usersExists := diffUsers(usersFromDb, usersFromSso)
	// Incremental users synchronize both user information and group information
	for _, user := range usersToBeAdd {
		if err = user.AddUserAndGroups(ctx, s.CoverTeams); err != nil {
			logger.Warningf("failed to sync add user[%v] to db, err: %v", *user, err)
		}
	}

	// Existing users synchronize group information only
	for _, user := range usersExists {
		if err = models.UserGroupMemberSyncByUser(ctx, user, s.CoverTeams); err != nil {
			logger.Warningf("failed to sync add user[%v] to db, err: %v", *user, err)
		}
	}

	var usersToBeDel []*models.User
	if s.SyncDel {
		usersToBeDel, _ = diffUsers(usersFromSso, usersFromDb)
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

	srs, err := lc.ldapReq(conn, lc.UserFilter)
	if err != nil {
		return nil, fmt.Errorf("ldap.error: ldap search fail: %v", err)
	}

	res := make(map[string]*models.User)

	for i := range srs {
		if srs[i] == nil {
			continue
		}
		for _, entry := range srs[i].Entries {
			attrs := lc.Attributes
			username := entry.GetAttributeValue(attrs.Username)
			nickname := entry.GetAttributeValue(attrs.Nickname)
			email := entry.GetAttributeValue(attrs.Email)
			phone := entry.GetAttributeValue(attrs.Phone)

			// Gets the roles and teams for this entry
			roleTeamMapping := lc.GetUserRolesAndTeams(entry)
			if len(roleTeamMapping.Roles) == 0 {
				// No role mapping is configured, the configured default role is used
				roleTeamMapping.Roles = lc.DefaultRoles
			}
			user := new(models.User)
			user.FullSsoFieldsWithTeams("ldap", username, nickname, phone, email, roleTeamMapping.Roles, roleTeamMapping.Teams)

			res[entry.GetAttributeValue(attrs.Username)] = user
		}
	}

	return res, nil
}

// newExtraUsers: in newUsers not in base
// updatedUsers: in newUsers and in base, update the user.TeamsLst data
func diffUsers(base, newUsers map[string]*models.User) (newExtraUsers, updatedUsers []*models.User) {
	for username, user := range newUsers {
		if baseUser, exist := base[username]; !exist {
			newExtraUsers = append(newExtraUsers, user)
		} else {
			if len(baseUser.TeamsLst) == 0 {
				// Need to pass on the team message
				baseUser.TeamsLst = user.TeamsLst
			}
			updatedUsers = append(updatedUsers, baseUser)
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

	srs, err := lc.ldapReq(conn, "(&(%s=%s))", lc.Attributes.Username, username)
	if err != nil {
		return false, err
	}

	for i := range srs {
		if srs[i] == nil {
			continue
		}

		if len(srs[i].Entries) > 0 {
			return true, nil
		}

	}

	return false, nil
}
