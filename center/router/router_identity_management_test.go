package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/toolkits/pkg/str"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func identityRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func seedIdentitySalt(t *testing.T, rt *Router) {
	t.Helper()
	db := rt.Ctx.DB
	require.NoError(t, db.Create(&models.Configs{Ckey: models.SALT, Cval: "test-salt"}).Error)
}

func identityRequest(t *testing.T, rt *Router, handler gin.HandlerFunc, body interface{}, id string,
	user *models.User, contextKeys ...string) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Set("username", "operator")
		for _, key := range contextKeys {
			switch key {
			case "busi_group":
				bg, err := models.BusiGroupGetById(rt.Ctx, 1)
				require.NoError(t, err)
				require.NotNil(t, bg)
				c.Set(key, bg)
			case "user_group":
				ug, err := models.UserGroupGetById(rt.Ctx, 1)
				require.NoError(t, err)
				require.NotNil(t, ug)
				c.Set(key, ug)
			}
		}
	})
	engine.POST("/objects/:id", handler)
	req := httptest.NewRequest(http.MethodPost, "/objects/"+id, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func identityReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Err string          `json:"err"`
		Dat json.RawMessage `json:"dat"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	if dat != nil && len(response.Dat) > 0 {
		require.NoError(t, json.Unmarshal(response.Dat, dat))
	}
	return response.Err
}

func TestUserAdd_HashesPasswordAndRecordsOperator(t *testing.T) {
	rt := identityRouter(t,
		&models.User{},
		&models.Configs{},
	)
	seedIdentitySalt(t, rt)
	form := userAddForm{Username: "alice", Password: "secret", Nickname: "Alice", Roles: []string{"Standard", "Viewer"}}

	w := identityRequest(t, rt, rt.userAddPost, form, "0", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, "alice", saved.Username)
	require.Equal(t, str.MD5("test-salt<-*Uk30^96eY*->secret"), saved.Password)
	require.Equal(t, "Standard Viewer", saved.Roles)
	require.Equal(t, "operator", saved.CreateBy)
	require.Equal(t, "operator", saved.UpdateBy)
}

func TestUserAdd_RejectsInvalidProfileAndDuplicateUsername(t *testing.T) {
	cases := []struct {
		name    string
		form    userAddForm
		message string
	}{
		{
			name: "empty roles", form: userAddForm{Username: "alice", Password: "secret", Roles: []string{}},
			message: "roles empty",
		},
		{
			name: "bad email", form: userAddForm{
				Username: "alice",
				Password: "secret",
				Roles: []string{
					"Viewer",
				},
				Email: "invalid",
			},
			message: "Email invalid",
		},
		{
			name: "duplicate", form: userAddForm{Username: "existing", Password: "secret", Roles: []string{"Viewer"}},
			message: "Username already exists",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t,
				&models.User{},
				&models.Configs{},
			)
			seedIdentitySalt(t, rt)
			require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "existing"}).Error)

			w := identityRequest(t, rt, rt.userAddPost, tc.form, "0", &models.User{Username: "operator"})

			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.User{}).Count(&count).Error)
			require.Equal(t, int64(1), count)
		})
	}
}

func TestUserProfilePut_PreservesDisabledStateAndPassword(t *testing.T) {
	rt := identityRouter(t,
		&models.User{},
		&models.Configs{},
	)
	original := models.User{Id: 1, Username: "alice", Password: "original-hash", Disabled: 1, Roles: "Viewer"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	form := userProfileForm{Nickname: "updated", Email: "alice@example.invalid", Roles: []string{"Standard"}}

	w := identityRequest(t, rt, rt.userProfilePut, form, "1", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, original.Disabled, saved.Disabled)
	require.Equal(t, original.Password, saved.Password)
	require.Equal(t, form.Nickname, saved.Nickname)
	require.Equal(t, "Standard", saved.Roles)
	require.Equal(t, "operator", saved.UpdateBy)
}

func TestUserProfilePutByService_SetsSuppliedPassword(t *testing.T) {
	rt := identityRouter(t,
		&models.User{},
		&models.Configs{},
	)
	seedIdentitySalt(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Password: "old", Roles: "Viewer"}).Error)
	body := map[string]interface{}{"password": "new-password", "roles": []string{"Viewer"}}

	w := identityRequest(t, rt, rt.userProfilePutByService, body, "1", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, str.MD5("test-salt<-*Uk30^96eY*->new-password"), saved.Password)
}

func TestUserProfilePutByService_KeepsPasswordWhenOmitted(t *testing.T) {
	rt := identityRouter(t,
		&models.User{},
		&models.Configs{},
	)
	seedIdentitySalt(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Password: "old", Roles: "Viewer"}).Error)
	body := map[string]interface{}{"nickname": "Alice", "roles": []string{"Standard"}}

	w := identityRequest(t, rt, rt.userProfilePutByService, body, "1", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	// Before the fix, every service update overwrote the password with the hash of "".
	require.Equal(t, "old", saved.Password)
	require.Equal(t, "Alice", saved.Nickname)
	require.Equal(t, "Standard", saved.Roles)
}

func TestUserPasswordPut_ChangesOnlyPasswordAndAuditFields(t *testing.T) {
	rt := identityRouter(t,
		&models.User{},
		&models.Configs{},
	)
	seedIdentitySalt(t, rt)
	original := models.User{Id: 1, Username: "alice", Password: "old", Disabled: 1, Roles: "Viewer", Nickname: "Alice"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	w := identityRequest(t, rt, rt.userPasswordPut, userPasswordForm{Password: "new-password"}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, str.MD5("test-salt<-*Uk30^96eY*->new-password"), saved.Password)
	require.Equal(t, original.Roles, saved.Roles)
	require.Equal(t, original.Disabled, saved.Disabled)
	require.Equal(t, original.Nickname, saved.Nickname)
	require.Equal(t, "operator", saved.UpdateBy)
}

func TestSelfPasswordPut_RequiresCorrectOldPassword(t *testing.T) {
	for _, correct := range []bool{true, false} {
		t.Run(fmt.Sprintf("correct=%t", correct), func(t *testing.T) {
			rt := identityRouter(t,
				&models.User{},
				&models.Configs{},
			)
			seedIdentitySalt(t, rt)
			original := models.User{Id: 1, Username: "alice", Password: str.MD5("test-salt<-*Uk30^96eY*->old")}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			oldpass := "old"
			if !correct {
				oldpass = "wrong"
			}

			w := identityRequest(t, rt, rt.selfPasswordPut, selfPasswordForm{OldPass: oldpass, NewPass: "new"}, "1", &original)

			var saved models.User
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			if correct {
				require.Empty(t, identityReply(t, w, nil))
				require.Equal(t, str.MD5("test-salt<-*Uk30^96eY*->new"), saved.Password)
				require.Equal(t, "alice", saved.UpdateBy)
			} else {
				require.Contains(t, identityReply(t, w, nil), "Incorrect old password")
				require.Equal(t, original.Password, saved.Password)
			}
		})
	}
}

func TestUserDelete_ProtectsLastEnabledAdmin(t *testing.T) {
	rt := identityRouter(t,
		&models.User{},
	)
	admin := models.User{Id: 1, Username: "admin", Roles: models.AdminRole}
	require.NoError(t, rt.Ctx.DB.Create(&admin).Error)

	w := identityRequest(t, rt, rt.userDel, nil, "1", nil)

	require.Contains(t, identityReply(t, w, nil), "cannot delete the last enabled admin")
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, admin.Username, saved.Username)
}

func TestUserDelete_RemovesMembershipAndTokens(t *testing.T) {
	rt := identityRouter(t,
		&models.User{},
		&models.UserGroupMember{},
		&models.UserToken{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Roles: "Viewer"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{UserId: 1, GroupId: 10}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserToken{Username: "alice", Token: "token"}).Error)

	w := identityRequest(t, rt, rt.userDel, nil, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	for _, model := range []interface{}{&models.User{}, &models.UserGroupMember{}, &models.UserToken{}} {
		var count int64
		require.NoError(t, rt.Ctx.DB.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestUserDisabledPut_RejectsInvalidStateWithoutWriting(t *testing.T) {
	rt := identityRouter(t,
		&models.User{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice"}).Error)

	w := identityRequest(t, rt, rt.userDisabledPut, userDisabledForm{Disabled: 2}, "1", &models.User{Id: 2})

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "disabled invalid")
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, models.UserEnabled, saved.Disabled)
}

func TestUserGroupAdd_AddsCreatorAsMember(t *testing.T) {
	rt := identityRouter(t,
		&models.UserGroup{},
		&models.UserGroupMember{},
		&models.Configs{},
	)
	user := &models.User{Id: 1, Username: "alice"}

	w := identityRequest(t, rt, rt.userGroupAdd, userGroupForm{Name: "oncall", Note: "primary"}, "0", user)

	var id int64
	require.Empty(t, identityReply(t, w, &id))
	var saved models.UserGroup
	require.NoError(t, rt.Ctx.DB.First(&saved, id).Error)
	require.Equal(t, "alice", saved.CreateBy)
	require.Equal(t, "oncall", saved.Name)
	ids, err := models.MemberIds(rt.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)
}

func TestUserGroupPut_RejectsDuplicateNameWithoutWriting(t *testing.T) {
	rt := identityRouter(t,
		&models.UserGroup{},
		&models.Configs{},
	)
	groups := []models.UserGroup{{Id: 1, Name: "original"}, {Id: 2, Name: "taken"}}
	require.NoError(t, rt.Ctx.DB.Create(&groups).Error)

	w := identityRequest(t, rt, rt.userGroupPut, userGroupForm{
		Name: "taken",
	}, "1", &models.User{
		Username: "editor",
	}, "user_group")

	require.Contains(t, identityReply(t, w, nil), "UserGroup already exists")
	var saved models.UserGroup
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "original", saved.Name)
}

func TestUserGroupMemberAdd_SkipsMissingUsersAndDuplicates(t *testing.T) {
	rt := identityRouter(t,
		&models.UserGroup{},
		&models.UserGroupMember{},
		&models.User{},
		&models.Configs{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 1, Name: "oncall"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 2, Username: "bob"}).Error)

	w := identityRequest(t, rt, rt.userGroupMemberAdd, idsForm{Ids: []int64{2, 2, 404}}, "1",
		&models.User{Username: "editor"}, "user_group")

	require.Empty(t, identityReply(t, w, nil))
	ids, err := models.MemberIds(rt.Ctx, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{2}, ids)
	var group models.UserGroup
	require.NoError(t, rt.Ctx.DB.First(&group, 1).Error)
	require.Equal(t, "editor", group.UpdateBy)
}

func TestUserGroupMemberDelete_RemovesOnlyRequestedMembers(t *testing.T) {
	rt := identityRouter(t,
		&models.UserGroup{},
		&models.UserGroupMember{},
		&models.Configs{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 1, Name: "oncall"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.UserGroupMember{{GroupId: 1, UserId: 2}, {GroupId: 1, UserId: 3}}).Error)

	w := identityRequest(t, rt, rt.userGroupMemberDel, idsForm{
		Ids: []int64{
			2,
		},
	}, "1", &models.User{
		Username: "editor",
	}, "user_group")

	require.Empty(t, identityReply(t, w, nil))
	ids, err := models.MemberIds(rt.Ctx, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{3}, ids)
}

func TestBusiGroupAdd_RequiresAtLeastOneWritableTeam(t *testing.T) {
	cases := []struct {
		name    string
		members []models.BusiGroupMember
		message string
	}{
		{name: "no members", message: "members empty"},
		{name: "read only", members: []models.BusiGroupMember{{UserGroupId: 10, PermFlag: "ro"}}, message: "rw permission"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t,
				&models.BusiGroup{},
			)

			w := identityRequest(t, rt, rt.busiGroupAdd, busiGroupForm{Name: "production", Members: tc.members}, "0", nil)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.BusiGroup{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestBusiGroupAdd_CreatesGroupAndTeamPermissions(t *testing.T) {
	rt := identityRouter(t,
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroup{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 10, Name: "oncall"}).Error)
	form := busiGroupForm{
		Name: "production", LabelEnable: 1, LabelValue: "prod",
		Members: []models.BusiGroupMember{{UserGroupId: 10, PermFlag: "rw"}},
	}

	w := identityRequest(t, rt, rt.busiGroupAdd, form, "0", nil)

	var id int64
	require.Empty(t, identityReply(t, w, &id))
	var saved models.BusiGroup
	require.NoError(t, rt.Ctx.DB.First(&saved, id).Error)
	require.Equal(t, "operator", saved.CreateBy)
	require.Equal(t, "prod", saved.LabelValue)
	members, err := models.BusiGroupMemberGetsByBusiGroupId(rt.Ctx, id)
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, int64(10), members[0].UserGroupId)
	require.Equal(t, "rw", members[0].PermFlag)
}

func TestBusiGroupMemberAdd_RejectsMismatchedGroupBeforeWriting(t *testing.T) {
	rt := identityRouter(t,
		&models.BusiGroup{},
		&models.BusiGroupMember{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroup{Id: 1, Name: "production"}).Error)
	members := []models.BusiGroupMember{{BusiGroupId: 1, UserGroupId: 10}, {BusiGroupId: 2, UserGroupId: 20}}

	w := identityRequest(t, rt, rt.busiGroupMemberAdd, members, "1", nil, "busi_group")

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "business group id invalid")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.BusiGroupMember{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestBusiGroupMemberDelete_ProtectsLastTeam(t *testing.T) {
	rt := identityRouter(t,
		&models.BusiGroup{},
		&models.BusiGroupMember{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroup{Id: 1, Name: "production"}).Error)
	member := models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: "rw"}
	require.NoError(t, rt.Ctx.DB.Create(&member).Error)

	w := identityRequest(t, rt, rt.busiGroupMemberDel, []models.BusiGroupMember{member}, "1", nil, "busi_group")

	require.Contains(t, identityReply(t, w, nil), "retain at least one team")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.BusiGroupMember{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestBusiGroupPut_DisablingLabelClearsLabelValue(t *testing.T) {
	rt := identityRouter(t,
		&models.BusiGroup{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroup{
		Id:          1,
		Name:        "production",
		LabelEnable: 1,
		LabelValue:  "prod",
	}).Error)

	w := identityRequest(t, rt, rt.busiGroupPut, busiGroupForm{
		Name:        "renamed",
		LabelEnable: 0,
		LabelValue:  "ignored",
	}, "1", nil, "busi_group")

	require.Empty(t, identityReply(t, w, nil))
	var saved models.BusiGroup
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "renamed", saved.Name)
	require.Zero(t, saved.LabelEnable)
	require.Empty(t, saved.LabelValue)
	require.Equal(t, "operator", saved.UpdateBy)
}

func TestRolePut_ProtectsAdminAndDuplicateNames(t *testing.T) {
	cases := []struct {
		name        string
		original    string
		replacement string
		message     string
	}{
		{name: "admin", original: "Admin", replacement: "Changed", message: "admin role can not be modified"},
		{name: "duplicate", original: "Viewer", replacement: "Operator", message: "role name already exists"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t,
				&models.Role{},
			)
			require.NoError(t, rt.Ctx.DB.Create(&[]models.Role{{Id: 1, Name: tc.original}, {Id: 2, Name: "Operator"}}).Error)

			w := identityRequest(t, rt, rt.rolePut, models.Role{Id: 1, Name: tc.replacement}, "1", nil)

			require.Contains(t, identityReply(t, w, nil), tc.message)
			var saved models.Role
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, tc.original, saved.Name)
		})
	}
}

func TestRoleBindOperation_ReplacesBindings(t *testing.T) {
	rt := identityRouter(t,
		&models.Role{},
		&models.RoleOperation{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.Role{Id: 1, Name: "Operator"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.RoleOperation{RoleName: "Operator", Operation: "/old"}).Error)

	w := identityRequest(t, rt, rt.roleBindOperation, []string{"/read", "/write"}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	ops, err := models.OperationsOfRole(rt.Ctx, []string{"Operator"})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"/read", "/write"}, ops)
}

func TestRoleBindOperation_ProtectsAdminBindings(t *testing.T) {
	rt := identityRouter(t,
		&models.Role{},
		&models.RoleOperation{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.Role{Id: 1, Name: "Admin"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.RoleOperation{RoleName: "Admin", Operation: "/keep"}).Error)

	w := identityRequest(t, rt, rt.roleBindOperation, []string{"/replace"}, "1", nil)

	require.Contains(t, identityReply(t, w, nil), "admin role can not be modified")
	ops, err := models.OperationsOfRole(rt.Ctx, []string{"Admin"})
	require.NoError(t, err)
	require.Equal(t, []string{"/keep"}, ops)
}

func TestUserGroupGet_ReturnsMembersAndBusinessGroups(t *testing.T) {
	rt := identityRouter(t,
		&models.UserGroup{},
		&models.UserGroupMember{},
		&models.User{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 1, Name: "oncall"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 2, Username: "bob"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 1, UserId: 2}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroup{Id: 3, Name: "production"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroupMember{BusiGroupId: 3, UserGroupId: 1, PermFlag: "rw"}).Error)

	w := identityRequest(t, rt, rt.userGroupGet, nil, "1", nil)

	var got struct {
		Users []models.User    `json:"users"`
		Group models.UserGroup `json:"user_group"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got.Users, 1)
	require.Equal(t, "bob", got.Users[0].Username)
	require.Len(t, got.Group.BusiGroups, 1)
	require.Equal(t, "production", got.Group.BusiGroups[0].Name)
}

func TestUserGroupPut_UpdatesNameAndNote(t *testing.T) {
	rt := identityRouter(t,
		&models.UserGroup{},
		&models.Configs{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 1, Name: "original", CreateBy: "creator"}).Error)

	w := identityRequest(t, rt, rt.userGroupPut, userGroupForm{Name: "renamed", Note: "updated"}, "1",
		&models.User{Username: "editor"}, "user_group")

	require.Empty(t, identityReply(t, w, nil))
	var saved models.UserGroup
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "renamed", saved.Name)
	require.Equal(t, "updated", saved.Note)
	require.Equal(t, "editor", saved.UpdateBy)
	require.Equal(t, "creator", saved.CreateBy)
}

func TestUserGroupDelete_RemovesMembers(t *testing.T) {
	rt := identityRouter(t,
		&models.UserGroup{},
		&models.UserGroupMember{},
		&models.Configs{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 1, Name: "oncall"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 1, UserId: 2}).Error)

	w := identityRequest(t, rt, rt.userGroupDel, nil, "1", nil, "user_group")

	require.Empty(t, identityReply(t, w, nil))
	for _, model := range []interface{}{&models.UserGroup{}, &models.UserGroupMember{}} {
		var count int64
		require.NoError(t, rt.Ctx.DB.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestBusiGroupGets_FiltersMembershipAndName(t *testing.T) {
	rt := identityRouter(t,
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.User{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BusiGroup{
		{Id: 1, Name: "prod-a"}, {Id: 2, Name: "prod-b"}, {Id: 3, Name: "stage"},
	}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)

	w := identityRequest(t, rt, rt.busiGroupGets, nil, "0?query=prod&limit=1", &models.User{Id: 1})

	var got []models.BusiGroup
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "prod-a", got[0].Name)
}

func TestBusiGroupMemberAdd_RecordsPermissionsAndOperator(t *testing.T) {
	rt := identityRouter(t,
		&models.BusiGroup{},
		&models.BusiGroupMember{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroup{Id: 1, Name: "production"}).Error)
	member := models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: "rw"}

	w := identityRequest(t, rt, rt.busiGroupMemberAdd, []models.BusiGroupMember{member}, "1", nil, "busi_group")

	require.Empty(t, identityReply(t, w, nil))
	members, err := models.BusiGroupMemberGetsByBusiGroupId(rt.Ctx, 1)
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, member.UserGroupId, members[0].UserGroupId)
	require.Equal(t, member.PermFlag, members[0].PermFlag)
	var saved models.BusiGroup
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "operator", saved.UpdateBy)
}

func TestRolePut_UpdatesEditableRole(t *testing.T) {
	rt := identityRouter(t,
		&models.Role{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.Role{Id: 1, Name: "Viewer"}).Error)

	w := identityRequest(t, rt, rt.rolePut, models.Role{Id: 1, Name: "Operator", Note: "can manage"}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var saved models.Role
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "Operator", saved.Name)
	require.Equal(t, "can manage", saved.Note)
}

func TestRoleDelete_ProtectsAdminAndDeletesEditableRoles(t *testing.T) {
	for _, admin := range []bool{true, false} {
		t.Run(fmt.Sprintf("admin=%t", admin), func(t *testing.T) {
			rt := identityRouter(t,
				&models.Role{},
			)
			name := "Viewer"
			if admin {
				name = "Admin"
			}
			require.NoError(t, rt.Ctx.DB.Create(&models.Role{Id: 1, Name: name}).Error)

			w := identityRequest(t, rt, rt.roleDel, nil, "1", nil)

			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.Role{}).Count(&count).Error)
			if admin {
				require.Contains(t, identityReply(t, w, nil), "admin role can not be modified")
				require.Equal(t, int64(1), count)
			} else {
				require.Empty(t, identityReply(t, w, nil))
				require.Zero(t, count)
			}
		})
	}
}

func TestRoleDelete_MissingRoleIsNoop(t *testing.T) {
	rt := identityRouter(t,
		&models.Role{},
	)

	w := identityRequest(t, rt, rt.roleDel, nil, "999", nil)

	require.Empty(t, identityReply(t, w, nil))
}

func TestAllPerms_GroupsOperationsByRole(t *testing.T) {
	rt := identityRouter(t,
		&models.Role{},
		&models.RoleOperation{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Role{{Name: "Viewer"}, {Name: "Operator"}}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.RoleOperation{
		{RoleName: "Viewer", Operation: "/read"},
		{RoleName: "Operator", Operation: "/read"},
		{RoleName: "Operator", Operation: "/write"},
	}).Error)

	w := identityRequest(t, rt, rt.allPerms, nil, "0", nil)

	var got map[string][]string
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []string{"/read"}, got["Viewer"])
	require.ElementsMatch(t, []string{"/read", "/write"}, got["Operator"])
}

func TestGetToken_ListsOnlyAuthenticatedUsersTokens(t *testing.T) {
	rt := identityRouter(t,
		&models.UserToken{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.UserToken{
		{Username: "operator", TokenName: "own", Token: "own-token"},
		{Username: "other", TokenName: "foreign", Token: "foreign-token"},
	}).Error)

	w := identityRequest(t, rt, rt.getToken, nil, "0", nil)

	var got []models.UserToken
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "own-token", got[0].Token)
}

func TestAddToken_RejectsDuplicateNames(t *testing.T) {
	rt := identityRouter(t,
		&models.UserToken{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserToken{
		Username:  "operator",
		TokenName: "existing",
		Token:     "keep",
	}).Error)

	w := identityRequest(t, rt, rt.addToken, tokenForm{TokenName: "existing", Token: "caller-token"}, "0", nil)

	require.Contains(t, identityReply(t, w, nil), "token name already exists")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.UserToken{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestUserGroupGetsByService_SelectsGroupsAndIncludesMembers(t *testing.T) {
	rt := identityRouter(t,
		&models.UserGroup{},
		&models.UserGroupMember{},
		&models.User{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.UserGroup{{Id: 1, Name: "oncall"}, {Id: 2, Name: "other"}}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 3, Username: "bob"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 1, UserId: 3}).Error)

	for _, tc := range []struct {
		name  string
		query string
		count int
	}{
		{name: "selected IDs", query: "?ids=1", count: 1},
		{name: "all groups", count: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, rt.userGroupGetsByService, nil, "0"+tc.query, nil)

			var got []models.UserGroup
			require.Empty(t, identityReply(t, w, &got))
			require.Len(t, got, tc.count)
			var oncall *models.UserGroup
			for i := range got {
				if got[i].Id == 1 {
					oncall = &got[i]
				}
			}
			require.NotNil(t, oncall)
			require.Equal(t, "oncall", oncall.Name)
			require.Len(t, oncall.Users, 1)
			require.Equal(t, "bob", oncall.Users[0].Username)
		})
	}
}

func TestUserGroupGets_FiltersMembershipAndName(t *testing.T) {
	rt := identityRouter(t,
		&models.UserGroup{},
		&models.UserGroupMember{},
		&models.User{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.UserGroup{
		{Id: 1, Name: "oncall-a"}, {Id: 2, Name: "oncall-b"}, {Id: 3, Name: "other", CreateBy: "alice"},
	}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 1, UserId: 1}).Error)

	w := identityRequest(t, rt, rt.userGroupGets, nil, "0?query=oncall&limit=1", &models.User{Id: 1, Username: "alice"})

	var got []models.UserGroup
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "oncall-a", got[0].Name)
}

func TestSelfProfilePut_PreservesRolesPasswordAndDisabledState(t *testing.T) {
	rt := identityRouter(t, &models.User{}, &models.Configs{})
	original := models.User{Id: 1, Username: "alice", Roles: "Viewer", Password: "keep-hash", Disabled: 1}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	form := selfProfileForm{Nickname: "updated", Email: "alice@example.invalid", Portrait: "avatar"}

	w := identityRequest(t, rt, rt.selfProfilePut, form, "0", &original)

	require.Empty(t, identityReply(t, w, nil))
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "updated", saved.Nickname)
	require.Equal(t, form.Email, saved.Email)
	require.Equal(t, form.Portrait, saved.Portrait)
	require.Equal(t, "Viewer", saved.Roles)
	require.Equal(t, "keep-hash", saved.Password)
	require.Equal(t, 1, saved.Disabled)
	require.Equal(t, "alice", saved.UpdateBy)
}
