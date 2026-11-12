package ldapx

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/require"
)

func TestReload_AppliesConnectionAndMappingConfiguration(t *testing.T) {
	client := New(Config{})
	t.Cleanup(client.Ticker.Stop)
	config := Config{
		Enable: true, Host: "ldap.internal", Port: 636, BaseDn: "dc=first|dc=second",
		BindUser: "cn=reader", BindPass: "secret", UserFilter: "(objectClass=person)", AuthFilter: "(uid=%s)",
		Attributes: LdapAttributes{Username: "uid", Nickname: "cn", Email: "mail", Phone: "telephoneNumber"},
		TLS:        true, StartTLS: false, SyncAddUsers: true, SyncDelUsers: true, SyncInterval: 60,
		CoverAttributes: true, CoverTeams: true, CoverRoles: true,
		DefaultRoles: []string{"viewer"}, DefaultTeams: []int64{10},
		RoleTeamMapping: []RoleTeamMapping{{DN: "cn=ops", Roles: []string{"admin"}, Teams: []int64{20}}},
	}

	client.Reload(config)

	require.True(t, client.Enable)
	require.Equal(t, config.Host, client.Host)
	require.Equal(t, config.Port, client.Port)
	require.Equal(t, config.BindUser, client.BindUser)
	require.Equal(t, config.BindPass, client.BindPass)
	require.Equal(t, []string{"dc=first", "dc=second"}, client.BaseDns)
	require.Equal(t, config.AuthFilter, client.AuthFilter)
	require.Equal(t, config.UserFilter, client.UserFilter)
	require.Equal(t, "memberOf", client.Attributes.Group)
	require.True(t, client.SyncAdd && client.SyncDel && client.TLS)
	require.True(t, client.CoverAttributes && client.CoverTeams && client.CoverRoles)
	require.Equal(t, config.DefaultRoles, client.DefaultRoles)
	require.Equal(t, config.DefaultTeams, client.DefaultTeams)
	require.Equal(t, config.RoleTeamMapping[0], client.RoleTeamMapping["cn=ops"])
}

func TestReload_DisablesClient(t *testing.T) {
	client := New(Config{Enable: true, Host: "ldap.internal"})
	t.Cleanup(client.Ticker.Stop)

	client.Reload(Config{Enable: false})

	require.False(t, client.Copy().Enable)
}

func TestCopy_IsolatesDefaultRolesAndTeams(t *testing.T) {
	client := New(Config{Enable: true, DefaultRoles: []string{"viewer"}, DefaultTeams: []int64{10}})
	t.Cleanup(client.Ticker.Stop)

	copied := client.Copy()
	copied.DefaultRoles[0] = "admin"
	copied.DefaultTeams[0] = 20

	require.Equal(t, []string{"viewer"}, client.DefaultRoles)
	require.Equal(t, []int64{10}, client.DefaultTeams)
}

func TestAttributeSearchList_UsesDefaultAndConfiguredAttributes(t *testing.T) {
	tests := []struct {
		name  string
		attrs LdapAttributes
		want  []string
	}{
		{name: "default username", want: []string{"uid"}},
		{
			name:  "configured attributes",
			attrs: LdapAttributes{Username: "account", Nickname: "cn", Email: "mail", Phone: "phone", Group: "memberOf"},
			want:  []string{"account", "cn", "mail", "phone", "memberOf"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &SsoClient{Attributes: tt.attrs}

			attributes := client.genLdapAttributeSearchList()

			require.Equal(t, tt.want, attributes)
		})
	}
}

func TestGetUserRolesAndTeams_MergesAndDeduplicatesDNMappings(t *testing.T) {
	client := New(Config{Enable: true, RoleTeamMapping: []RoleTeamMapping{
		{DN: "cn=ops", Roles: []string{"viewer", "admin"}, Teams: []int64{10, 20}},
		{DN: "uid=alice", Roles: []string{"admin"}, Teams: []int64{20, 30}},
	}})
	t.Cleanup(client.Ticker.Stop)
	entry := ldap.NewEntry("uid=alice", map[string][]string{"memberOf": {"cn=ops", "cn=unknown"}})

	mapping := client.GetUserRolesAndTeams(entry)

	require.Equal(t, "uid=alice", mapping.DN)
	require.ElementsMatch(t, []string{"viewer", "admin"}, mapping.Roles)
	require.ElementsMatch(t, []int64{10, 20, 30}, mapping.Teams)
}

func TestGetUserRolesAndTeams_UnmappedEntryHasNoAssignments(t *testing.T) {
	client := &SsoClient{Attributes: LdapAttributes{Group: "memberOf"}, DefaultRoles: []string{"viewer"}}

	mapping := client.GetUserRolesAndTeams(ldap.NewEntry("uid=unknown", nil))

	// Defaults are applied by login/sync, rather than by the DN mapping helper.
	require.Equal(t, "uid=unknown", mapping.DN)
	require.Empty(t, mapping.Roles)
	require.Empty(t, mapping.Teams)
}

func TestDiffUsers_SeparatesNewUsersAndPreservesDatabaseIdentity(t *testing.T) {
	existing := &models.User{Id: 7, Username: "alice"}
	added := &models.User{Username: "bob", TeamsLst: []int64{20}}
	base := map[string]*models.User{"alice": existing, "removed": {Id: 8, Username: "removed"}}
	incoming := map[string]*models.User{
		"alice": {Username: "alice", TeamsLst: []int64{10}}, "bob": added,
	}

	extra, updated := diffUsers(base, incoming)

	require.Equal(t, []*models.User{added}, extra)
	require.Equal(t, []*models.User{existing}, updated)
	require.Equal(t, int64(7), updated[0].Id)
	require.Equal(t, []int64{10}, updated[0].TeamsLst)
}

func TestDiffUsers_PreservesExistingTeamAssignments(t *testing.T) {
	existing := &models.User{Id: 7, Username: "alice", TeamsLst: []int64{10}}

	extra, updated := diffUsers(map[string]*models.User{"alice": existing}, map[string]*models.User{
		"alice": {Username: "alice", TeamsLst: []int64{20}},
	})

	require.Empty(t, extra)
	require.Equal(t, []int64{10}, updated[0].TeamsLst)
}
