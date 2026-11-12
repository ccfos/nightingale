package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRoleAdd_RejectsDuplicatesWithoutChangingExistingRole(t *testing.T) {
	rt := identityRouter(t, &models.Role{})
	original := models.Role{Id: 1, Name: "Operator", Note: "original"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	w := identityRequest(t, rt, rt.roleAdd, models.Role{Name: "Operator", Note: "changed"}, "1", nil)

	require.Contains(t, identityReply(t, w, nil), "role name already exists")
	var got []models.Role
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Equal(t, []models.Role{original}, got)
}

func TestRoleAdd_SavesNewRoleAndListsIt(t *testing.T) {
	rt := identityRouter(t, &models.Role{})

	w := identityRequest(t, rt, rt.roleAdd, models.Role{Name: "Operator", Note: "write access"}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	for _, handler := range []gin.HandlerFunc{rt.rolesGets, rt.roleGets} {
		read := identityRequest(t, rt, handler, nil, "1", nil)
		var got []models.Role
		require.Empty(t, identityReply(t, read, &got))
		require.Len(t, got, 1)
		require.Equal(t, "Operator", got[0].Name)
		require.Equal(t, "write access", got[0].Note)
	}
}

func TestPermsGets_CombinesUserRolesAndExpandsAdminOperations(t *testing.T) {
	rt := identityRouter(t, &models.RoleOperation{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.RoleOperation{
		{RoleName: "Viewer", Operation: "/boards/read"},
		{RoleName: "Operator", Operation: "/boards/write"},
		{RoleName: "Other", Operation: "/users/write"},
	}).Error)
	previous := cconf.Operations
	cconf.Operations = cconf.Operation{Ops: []cconf.Ops{{
		Name: "boards", Ops: []cconf.SingleOp{{Name: "/boards/read"}, {Name: "/boards/write"}},
	}}}
	t.Cleanup(func() { cconf.Operations = previous })
	cases := []struct {
		name string
		user models.User
	}{
		{name: "combined roles", user: models.User{Roles: "Viewer Operator"}},
		{name: "admin", user: models.User{RolesLst: []string{"Admin"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, rt.permsGets, nil, "1", &tc.user)

			var got []string
			require.Empty(t, identityReply(t, w, &got))
			require.ElementsMatch(t, []string{"/boards/read", "/boards/write"}, got)
		})
	}
}

func TestOperationOfRole_UsesBoundOperationsAndAdminRegistry(t *testing.T) {
	rt := identityRouter(t, &models.Role{}, &models.RoleOperation{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Role{{Id: 1, Name: "Viewer"}, {Id: 2, Name: "Admin"}}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.RoleOperation{RoleName: "Viewer", Operation: "/boards/read"}).Error)
	previous := cconf.Operations
	cconf.Operations = cconf.Operation{Ops: []cconf.Ops{{Ops: []cconf.SingleOp{{Name: "/all"}}}}}
	t.Cleanup(func() { cconf.Operations = previous })
	cases := []struct {
		name     string
		id       string
		expected []string
	}{
		{name: "viewer", id: "1", expected: []string{"/boards/read"}},
		{name: "admin", id: "2", expected: []string{"/all"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, rt.operationOfRole, nil, tc.id, nil)

			var got []string
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, tc.expected, got)
		})
	}
	missing := identityRequest(t, rt, rt.operationOfRole, nil, "404", nil)
	require.Contains(t, identityReply(t, missing, nil), "role not found")
}

func TestOperations_RendersConfiguredGroupsWithoutDroppingOperationNames(t *testing.T) {
	rt := &Router{}
	rt.Operations = cconf.Operation{Ops: []cconf.Ops{{
		Name: "boards", Cname: "Dashboards", Ops: []cconf.SingleOp{{Name: "/boards/read", Cname: "Read dashboards"}},
	}}}

	w := identityRequest(t, rt, rt.operations, nil, "1", nil)

	var got []cconf.Ops
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, rt.Operations.Ops, got)
}
