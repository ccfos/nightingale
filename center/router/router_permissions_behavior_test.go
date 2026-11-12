package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func permissionRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func seedPermissionMembership(t *testing.T, rt *Router) {
	t.Helper()
	db := rt.Ctx.DB
	require.NoError(t, db.Create(&models.BusiGroup{Id: 10, Name: "production"}).Error)
	require.NoError(t, db.Create(&models.BusiGroupMember{BusiGroupId: 10, UserGroupId: 20, PermFlag: "ro"}).Error)
	require.NoError(t, db.Create(&models.BusiGroupMember{BusiGroupId: 10, UserGroupId: 21, PermFlag: "rw"}).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 20, UserId: 1}).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 21, UserId: 2}).Error)
}

func permissionRequest(user *models.User, middleware gin.HandlerFunc, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	if user.Roles != "" {
		user.RolesLst = strings.Fields(user.Roles)
	}
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Set("userid", user.Id)
		c.Next()
	})
	engine.PUT("/groups/:id", middleware, handler)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/groups/10", strings.NewReader(`{}`)))
	return w
}

func TestBgrwCheck_RequiresWritableMembership(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		status int
	}{
		{name: "writer", user: models.User{Id: 2, Username: "writer"}, status: http.StatusOK},
		{name: "reader", user: models.User{Id: 1, Username: "reader"}, status: http.StatusForbidden},
		{name: "outsider", user: models.User{Id: 3, Username: "outsider"}, status: http.StatusForbidden},
		{name: "admin", user: models.User{Id: 4, Roles: models.AdminRole}, status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := permissionRouter(t,
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedPermissionMembership(t, rt)

			w := permissionRequest(&tc.user, func(c *gin.Context) { rt.bgrwCheck(c, 10) }, func(c *gin.Context) {
				bg := c.MustGet("busi_group").(*models.BusiGroup)
				require.Equal(t, int64(10), bg.Id)
				require.NoError(t, rt.Ctx.DB.Model(bg).Update("name", "updated").Error)
				c.Status(http.StatusOK)
			})

			require.Equal(t, tc.status, w.Code, w.Body.String())
			var bg models.BusiGroup
			require.NoError(t, rt.Ctx.DB.First(&bg, 10).Error)
			if tc.status == http.StatusOK {
				require.Equal(t, "updated", bg.Name)
			} else {
				require.Contains(t, w.Body.String(), "forbidden")
				require.Equal(t, "production", bg.Name)
			}
		})
	}
}

func TestBgroCheck_AllowsReadersAndRejectsOutsiders(t *testing.T) {
	cases := []struct {
		name   string
		uid    int64
		status int
	}{
		{name: "reader", uid: 1, status: http.StatusOK},
		{name: "writer", uid: 2, status: http.StatusOK},
		{name: "outsider", uid: 3, status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := permissionRouter(t,
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedPermissionMembership(t, rt)

			w := permissionRequest(&models.User{Id: tc.uid}, func(c *gin.Context) { rt.bgroCheck(c, 10) },
				func(c *gin.Context) { c.JSON(http.StatusOK, c.MustGet("busi_group")) })

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				require.Contains(t, w.Body.String(), `"name":"production"`)
			} else {
				require.Contains(t, w.Body.String(), "forbidden")
			}
		})
	}
}

func TestBusiGroupMiddleware_PopulatesAuthorizedGroup(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(map[bool]string{false: "read", true: "write"}[write], func(t *testing.T) {
			rt := permissionRouter(t,
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedPermissionMembership(t, rt)
			middleware := rt.bgro()
			if write {
				middleware = rt.Bgrw()
			}

			w := permissionRequest(&models.User{Id: 2}, middleware, func(c *gin.Context) {
				c.JSON(http.StatusOK, c.MustGet("busi_group"))
			})

			require.Equal(t, http.StatusOK, w.Code)
			require.Contains(t, w.Body.String(), `"id":10`)
			denied := permissionRequest(&models.User{Id: 3}, middleware, func(c *gin.Context) {
				t.Fatal("unauthorized middleware continued")
			})
			require.Equal(t, http.StatusForbidden, denied.Code)
		})
	}
}

func TestBgrwChecks_RejectsMissingGroupBeforeMutation(t *testing.T) {
	rt := permissionRouter(t,
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedPermissionMembership(t, rt)

	w := permissionRequest(&models.User{Roles: models.AdminRole}, func(c *gin.Context) {
		rt.bgrwChecks(c, []int64{10, 10, 404})
	}, func(c *gin.Context) { t.Fatal("missing group reached handler") })

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "No such BusiGroup")
	var bg models.BusiGroup
	require.NoError(t, rt.Ctx.DB.First(&bg, 10).Error)
	require.Equal(t, "production", bg.Name)
}

func TestBgroCheckAllowMissing_RestrictsOrphanedObjectsToAdmins(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		gid    int64
		status int
	}{
		{name: "admin orphan", user: models.User{Roles: models.AdminRole}, gid: 404, status: http.StatusOK},
		{name: "reader orphan", user: models.User{Id: 1}, gid: 404, status: http.StatusForbidden},
		{name: "reader existing", user: models.User{Id: 1}, gid: 10, status: http.StatusOK},
		{name: "outsider existing", user: models.User{Id: 3}, gid: 10, status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := permissionRouter(t,
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedPermissionMembership(t, rt)

			w := permissionRequest(&tc.user, func(c *gin.Context) { rt.bgroCheckAllowMissing(c, tc.gid) },
				func(c *gin.Context) { c.String(http.StatusOK, "read allowed") })

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				require.Equal(t, "read allowed", w.Body.String())
			}
		})
	}
}

func TestAdminMiddleware_RejectsNonAdminsAndMissingAccounts(t *testing.T) {
	cases := []struct {
		name   string
		roles  string
		exists bool
		status int
	}{
		{name: "admin", roles: "Viewer Admin", exists: true, status: http.StatusOK},
		{name: "similar role", roles: "SuperAdmin", exists: true, status: http.StatusForbidden},
		{name: "viewer", roles: "Viewer", exists: true, status: http.StatusForbidden},
		{name: "deleted", exists: false, status: http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := permissionRouter(t,
				&models.User{},
			)
			user := &models.User{Id: 9, Username: "operator", Roles: tc.roles}
			if tc.exists {
				require.NoError(t, rt.Ctx.DB.Create(user).Error)
			}

			w := permissionRequest(user, rt.admin(), func(c *gin.Context) {
				require.Equal(t, tc.roles, c.MustGet("user").(*models.User).Roles)
				c.String(http.StatusOK, "admin allowed")
			})

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				require.Equal(t, "admin allowed", w.Body.String())
			}
		})
	}
}

func TestPermMiddleware_RequiresBoundOperation(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		status int
	}{
		{name: "bound role", user: models.User{RolesLst: []string{"Operator"}}, status: http.StatusOK},
		{name: "unbound role", user: models.User{RolesLst: []string{"Viewer"}}, status: http.StatusForbidden},
		{name: "admin", user: models.User{Roles: models.AdminRole}, status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := permissionRouter(t,
				&models.RoleOperation{},
			)
			require.NoError(t, rt.Ctx.DB.Create(&models.RoleOperation{RoleName: "Operator", Operation: "/rules/write"}).Error)

			w := permissionRequest(&tc.user, rt.Perm("/rules/write"), func(c *gin.Context) {
				c.String(http.StatusOK, "operation allowed")
			})

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				require.Equal(t, "operation allowed", w.Body.String())
			}
		})
	}
}

func TestUserGroupWrite_AllowsCreatorMemberOrAdmin(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		status int
	}{
		{name: "creator", user: models.User{Username: "creator"}, status: http.StatusOK},
		{name: "member", user: models.User{Id: 2}, status: http.StatusOK},
		{name: "admin", user: models.User{Roles: models.AdminRole}, status: http.StatusOK},
		{name: "outsider", user: models.User{Id: 3}, status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := permissionRouter(t,
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
				&models.UserGroup{},
			)
			require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 10, Name: "oncall", CreateBy: "creator"}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 10, UserId: 2}).Error)

			w := permissionRequest(&tc.user, rt.userGroupWrite(), func(c *gin.Context) {
				ug := c.MustGet("user_group").(*models.UserGroup)
				require.NoError(t, rt.Ctx.DB.Model(ug).Update("note", "edited").Error)
				c.Status(http.StatusOK)
			})

			require.Equal(t, tc.status, w.Code)
			var ug models.UserGroup
			require.NoError(t, rt.Ctx.DB.First(&ug, 10).Error)
			if tc.status == http.StatusOK {
				require.Equal(t, "edited", ug.Note)
			} else {
				require.Empty(t, ug.Note)
			}
		})
	}
}
