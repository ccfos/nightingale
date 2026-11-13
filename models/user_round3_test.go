package models

import (
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/stretchr/testify/require"
)

func TestUserUpdateSsoFields_PreservesMissingAttributesAndOptionalRoles(t *testing.T) {
	user := User{
		Nickname: "original", Phone: "123", Email: "old@example.com", Roles: "viewer", RolesLst: []string{"viewer"},
	}

	fields := user.UpdateSsoFieldsWithRoles("oidc", "renamed", "", "", nil)

	require.ElementsMatch(t, []interface{}{"nickname", "phone", "email", "update_by", "belong"}, fields)
	require.Equal(t, "renamed", user.Nickname)
	require.Equal(t, "123", user.Phone)
	require.Equal(t, "old@example.com", user.Email)
	require.Equal(t, "oidc", user.Belong)
	require.Equal(t, "oidc", user.UpdateBy)
	require.Equal(t, "viewer", user.Roles)
	fields = user.UpdateSsoFieldsWithRoles("ldap", "", "456", "new@example.com", []string{"viewer", "editor"})
	require.Contains(t, fields, "roles")
	require.Equal(t, "viewer editor", user.Roles)
	require.Equal(t, []string{"viewer", "editor"}, user.RolesLst)
	require.Equal(t, "456", user.Phone)
	require.Equal(t, "new@example.com", user.Email)
}

func TestUserFullSsoFieldsWithTeams_InitializesExternalIdentity(t *testing.T) {
	user := User{Portrait: "old", Password: "old"}

	user.FullSsoFieldsWithTeams("oidc", "alice", "Alice", "123", "alice@example.com", []string{"viewer"}, []int64{7, 8})

	require.Equal(t, "alice", user.Username)
	require.Equal(t, "Alice", user.Nickname)
	require.Equal(t, "123", user.Phone)
	require.Equal(t, "alice@example.com", user.Email)
	require.Equal(t, "******", user.Password)
	require.Empty(t, user.Portrait)
	require.JSONEq(t, "{}", string(user.Contacts))
	require.Equal(t, "viewer", user.Roles)
	require.Equal(t, []string{"viewer"}, user.RolesLst)
	require.Equal(t, []int64{7, 8}, user.TeamsLst)
	require.Equal(t, "oidc", user.CreateBy)
	require.Equal(t, "oidc", user.UpdateBy)
	require.Equal(t, "oidc", user.Belong)
	require.Equal(t, user.CreateAt, user.UpdateAt)
}

func TestUserAdd_RejectsDuplicateUsernameAndPersistsAudit(t *testing.T) {
	c := pipelineContext(t, &User{})
	user := User{Username: "alice", Nickname: "Alice", Roles: "viewer"}

	require.NoError(t, user.Add(c))
	duplicate := User{Username: "alice", Nickname: "imposter"}
	require.ErrorContains(t, duplicate.Add(c), "Username already exists")

	got, err := UserGetByUsername(c, "alice")
	require.NoError(t, err)
	require.Equal(t, "Alice", got.Nickname)
	require.Equal(t, user.CreateAt, got.UpdateAt)
	require.Equal(t, []string{"viewer"}, got.RolesLst)
	missing, err := UserGetByUsername(c, "missing")
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestUserUpdateAllFields_PreservesConcurrentDisabledState(t *testing.T) {
	c := pipelineContext(t, &User{})
	stored := User{Id: 1, Username: "alice", Disabled: UserDisabled, Nickname: "old"}
	require.NoError(t, c.DB.Create(&stored).Error)
	stale := User{Id: 1, Username: "alice", Disabled: UserEnabled, Nickname: "new", Contacts: ormx.JSONObj(`{}`)}

	require.NoError(t, stale.UpdateAllFields(c))

	got, err := UserGetById(c, 1)
	require.NoError(t, err)
	require.Equal(t, "new", got.Nickname)
	require.Equal(t, UserDisabled, got.Disabled)
}

func TestUserChangePassword_RejectsWrongOldPasswordAndPersistsNewHash(t *testing.T) {
	c := pipelineContext(t, &User{}, &Configs{})
	require.NoError(t, c.DB.Create(&Configs{Ckey: SALT, Cval: "test-salt"}).Error)
	hash, err := CryptoPass(c, "old-password")
	require.NoError(t, err)
	user := User{Username: "alice", Password: hash}
	require.NoError(t, user.Add(c))

	require.EqualError(t, user.ChangePassword(c, "wrong", "new-password"), "Incorrect old password")
	before, err := UserGetById(c, user.Id)
	require.NoError(t, err)
	require.Equal(t, hash, before.Password)
	require.NoError(t, user.ChangePassword(c, "old-password", "new-password"))

	got, err := UserGetById(c, user.Id)
	require.NoError(t, err)
	expected, err := CryptoPass(c, "new-password")
	require.NoError(t, err)
	require.Equal(t, expected, got.Password)
	require.NotEqual(t, hash, got.Password)
	require.Equal(t, "alice", got.UpdateBy)
}

func TestUserChangePassword_RejectsSSOPlaceholderPassword(t *testing.T) {
	user := User{Username: "alice", Belong: "oidc", Password: "******"}

	err := user.ChangePassword(nil, "old", "new")

	require.ErrorContains(t, err, "cannot change password locally")
	require.ErrorContains(t, err, "oidc")
}

func TestPassLogin_AuthenticatesLocalUserAndRejectsDisabledOrInvalidCredentials(t *testing.T) {
	cases := []struct {
		name     string
		username string
		password string
		disabled int
		message  string
	}{
		{name: "valid", username: "alice", password: "correct"},
		{name: "missing", username: "missing", password: "correct", message: "Username or password invalid"},
		{name: "wrong password", username: "alice", password: "wrong", message: "Username or password invalid"},
		{name: "disabled", username: "alice", password: "correct", disabled: UserDisabled, message: ErrUserDisabled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &User{}, &Configs{})
			require.NoError(t, c.DB.Create(&Configs{Ckey: SALT, Cval: "test-salt"}).Error)
			hash, err := CryptoPass(c, "correct")
			require.NoError(t, err)
			require.NoError(t, c.DB.Create(&User{Id: 1, Username: "alice", Password: hash, Disabled: tc.disabled}).Error)

			got, err := PassLogin(c, nil, tc.username, tc.password)

			if tc.message != "" {
				require.EqualError(t, err, tc.message)
				require.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, int64(1), got.Id)
			}
		})
	}
}

func TestPassLogin_EnforcesRedisFailureLimit(t *testing.T) {
	c := pipelineContext(t, &User{}, &Configs{})
	rds := newTestRedis(t)
	require.NoError(t, c.DB.Create(&[]Configs{
		{Ckey: SALT, Cval: "test-salt"}, {Ckey: "login_fail_count", Cval: "300 2"},
	}).Error)
	hash, err := CryptoPass(c, "correct")
	require.NoError(t, err)
	require.NoError(t, c.DB.Create(&User{Username: "alice", Password: hash}).Error)

	for i := 0; i < 2; i++ {
		got, err := PassLogin(c, rds, "alice", "wrong")
		require.EqualError(t, err, "Username or password invalid")
		require.Nil(t, got)
	}
	got, err := PassLogin(c, rds, "alice", "correct")

	require.EqualError(t, err, "reach login fail count")
	require.Nil(t, got)
	count, err := rds.Get(c.GetContext(), "/userlogin/errorcount/alice").Result()
	require.NoError(t, err)
	require.Equal(t, "2", count)
}

func TestPassLogin_IgnoresMalformedFailureLimitConfiguration(t *testing.T) {
	for _, value := range []string{"300", "bad 2", "300 bad"} {
		t.Run(value, func(t *testing.T) {
			c := pipelineContext(t, &User{}, &Configs{})
			require.NoError(t, c.DB.Create(&[]Configs{
				{Ckey: SALT, Cval: "test-salt"}, {Ckey: "login_fail_count", Cval: value},
			}).Error)
			hash, err := CryptoPass(c, "correct")
			require.NoError(t, err)
			require.NoError(t, c.DB.Create(&User{Username: "alice", Password: hash}).Error)

			got, err := PassLogin(c, nil, "alice", "correct")

			require.NoError(t, err)
			require.Equal(t, "alice", got.Username)
		})
	}
}

func TestLoginFailureCount_ReportsInvalidRedisData(t *testing.T) {
	c := pipelineContext(t, &User{})
	rds := newTestRedis(t)
	require.NoError(t, rds.Set(c.GetContext(), "/userlogin/errorcount/alice", "invalid", 0).Err())

	reached, err := reachLoginFailCount(c, rds, "alice", 2)

	require.Error(t, err)
	require.False(t, reached)
	incrLoginFailCount(c, rds, "alice", 300)
	reached, err = reachLoginFailCount(c, rds, "alice", 2)
	require.NoError(t, err)
	require.False(t, reached)
	count, err := rds.Get(c.GetContext(), "/userlogin/errorcount/alice").Result()
	require.NoError(t, err)
	require.Equal(t, "1", count)
}

func TestUserStatusById_DistinguishesMissingEnabledAndDisabled(t *testing.T) {
	c := pipelineContext(t, &User{})
	require.NoError(t, c.DB.Create(&[]User{{Id: 1}, {Id: 2, Disabled: UserDisabled}}).Error)
	cases := []struct {
		name     string
		id       int64
		exists   bool
		disabled bool
	}{
		{name: "enabled", id: 1, exists: true},
		{name: "disabled", id: 2, exists: true, disabled: true},
		{name: "missing", id: 99},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exists, disabled, err := UserStatusById(c, tc.id)

			require.NoError(t, err)
			require.Equal(t, tc.exists, exists)
			require.Equal(t, tc.disabled, disabled)
		})
	}
}

func TestUserDel_RemovesMembershipsAndTokensOnlyForDeletedUser(t *testing.T) {
	c := pipelineContext(t, &User{}, &UserGroupMember{}, &UserToken{})
	user := User{Id: 1, Username: "alice"}
	require.NoError(t, c.DB.Create(&[]User{user, {Id: 2, Username: "bob"}}).Error)
	require.NoError(t, c.DB.Create(&[]UserGroupMember{{GroupId: 7, UserId: 1}, {GroupId: 7, UserId: 2}}).Error)
	require.NoError(t, c.DB.Create(&[]UserToken{{Username: "alice", Token: "a"}, {Username: "bob", Token: "b"}}).Error)

	require.NoError(t, user.Del(c))

	missing, err := UserGetById(c, 1)
	require.NoError(t, err)
	require.Nil(t, missing)
	members, err := MemberIds(c, 7)
	require.NoError(t, err)
	require.Equal(t, []int64{2}, members)
	var tokens []UserToken
	require.NoError(t, c.DB.Find(&tokens).Error)
	require.Len(t, tokens, 1)
	require.Equal(t, "bob", tokens[0].Username)
}

func TestUserDel_ProtectsLastEnabledAdmin(t *testing.T) {
	c := pipelineContext(t, &User{}, &UserGroupMember{}, &UserToken{})
	admin := User{Id: 1, Username: "admin", Roles: AdminRole}
	require.NoError(t, c.DB.Create(&admin).Error)

	err := admin.Del(c)

	require.Error(t, err)
	got, err := UserGetById(c, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, got.Admin)
}

func TestUserNicknameMap_DeduplicatesNamesAndOmitsMissingUsers(t *testing.T) {
	c := pipelineContext(t, &User{})
	require.NoError(t, c.DB.Create(&[]User{
		{Username: "alice", Nickname: "Alice"}, {Username: "bob", Nickname: "Bob"},
	}).Error)

	got := UserNicknameMap(c, []string{"alice", "", "alice", "missing"})

	require.Equal(t, map[string]string{"alice": "Alice"}, got)
	require.Empty(t, UserNicknameMap(c, nil))
	require.Empty(t, UserNicknameMap(c, []string{"", ""}))
}

func TestFillUpdateByNicknames_HandlesValueAndPointerSlices(t *testing.T) {
	c := pipelineContext(t, &User{})
	require.NoError(t, c.DB.Create(&User{Username: "alice", Nickname: "Alice"}).Error)
	values := []SavedView{{UpdateBy: "alice"}, {UpdateBy: "missing", UpdateByNickname: "stale"}}
	pointers := []*TaskTpl{{UpdateBy: "alice"}, nil, {UpdateBy: "missing", UpdateByNickname: "stale"}}

	FillUpdateByNicknames(c, values)
	FillUpdateByNicknames(c, pointers)

	require.Equal(t, "Alice", values[0].UpdateByNickname)
	require.Empty(t, values[1].UpdateByNickname)
	require.Equal(t, "Alice", pointers[0].UpdateByNickname)
	require.Nil(t, pointers[1])
	require.Empty(t, pointers[2].UpdateByNickname)
}

func TestUsersGetByGroupIds_ResolvesMembersInUsernameOrder(t *testing.T) {
	c := pipelineContext(t, &User{}, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&[]User{
		{Id: 1, Username: "zulu", Roles: "viewer"}, {Id: 2, Username: "alpha", Roles: AdminRole},
		{Id: 3, Username: "excluded"},
	}).Error)
	require.NoError(t, c.DB.Create(&[]UserGroupMember{
		{GroupId: 7, UserId: 1}, {GroupId: 7, UserId: 2}, {GroupId: 8, UserId: 3},
	}).Error)

	got, err := UsersGetByGroupIds(c, []int64{7})

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "alpha", got[0].Username)
	require.True(t, got[0].Admin)
	require.Equal(t, []string{"viewer"}, got[1].RolesLst)
	empty, err := UsersGetByGroupIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
	empty, err = UserGetsByIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
