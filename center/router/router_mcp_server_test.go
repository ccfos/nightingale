package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func nonAdmin(id int64) *models.User { return &models.User{Id: id} }
func admin(id int64) *models.User {
	return &models.User{Id: id, RolesLst: []string{models.AdminRole}}
}

func publicServer() *models.MCPServer { return &models.MCPServer{Private: 0} }
func privateServer(teams ...int64) *models.MCPServer {
	return &models.MCPServer{Private: 1, UserGroupIds: teams}
}

// TestMCPCanManage pins the management predicate: admins always; others need a
// team in common with the server's owners.
func TestMCPCanManage(t *testing.T) {
	cases := []struct {
		name string
		me   *models.User
		gids []int64
		obj  *models.MCPServer
		want bool
	}{
		{"admin manages any", admin(1), nil, privateServer(10), true},
		{"member manages (team intersects)", nonAdmin(2), []int64{10}, privateServer(10), true},
		{"non-member cannot manage", nonAdmin(3), []int64{20}, privateServer(10), false},
		{"no teams cannot manage owned server", nonAdmin(4), nil, privateServer(10), false},
		// A public server with no owners is manageable only by admins — a
		// non-admin has no team to intersect with an empty owner list.
		{"non-admin cannot manage ownerless public", nonAdmin(5), []int64{10}, publicServer(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mcpCanManage(tc.me, tc.gids, tc.obj); got != tc.want {
				t.Fatalf("mcpCanManage = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMCPCanUse covers the four scenarios that gate whether a chatting user gets
// a bound MCP server's tools: public → everyone; private → managers only.
func TestMCPCanUse(t *testing.T) {
	cases := []struct {
		name string
		me   *models.User
		gids []int64
		obj  *models.MCPServer
		want bool
	}{
		{"public usable by non-member", nonAdmin(1), []int64{20}, publicServer(), true},
		{"private dropped for non-member", nonAdmin(2), []int64{20}, privateServer(10), false},
		{"private usable by admin", admin(3), nil, privateServer(10), true},
		{"private usable by owning-team member", nonAdmin(4), []int64{10}, privateServer(10), true},
		// Defensive: no resolved user → private is never usable, public still is.
		{"nil user cannot use private", nil, nil, privateServer(10), false},
		{"nil user can use public", nil, nil, publicServer(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mcpCanUse(tc.me, tc.gids, tc.obj); got != tc.want {
				t.Fatalf("mcpCanUse = %v, want %v", got, tc.want)
			}
		})
	}
}
