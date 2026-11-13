package flashduty

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type dutyRequest struct {
	Method      string
	Path        string
	AppKey      string
	ContentType string
	Auth        string
	Body        map[string]interface{}
}

func dutyServer(t *testing.T, responses map[string]string) <-chan dutyRequest {
	t.Helper()
	requests := make(chan dutyRequest, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Error(err)
		}
		requests <- dutyRequest{
			Method: r.Method, Path: r.URL.Path, AppKey: r.URL.Query().Get("app_key"),
			ContentType: r.Header.Get("Content-Type"), Auth: r.Header.Get("X-Test-Auth"), Body: decoded,
		}
		response, ok := responses[r.URL.Path]
		if !ok {
			response = `{"data":{}}`
		}
		_, _ = io.WriteString(w, response)
	}))
	oldAPI, oldHeaders, oldTimeout := Api, Headers, Timeout
	t.Cleanup(func() { server.Close(); Api, Headers, Timeout = oldAPI, oldHeaders, oldTimeout })
	Init(cconf.FlashDuty{Api: server.URL, Headers: map[string]string{"X-Test-Auth": "secret"}, Timeout: 1000})
	return requests
}

func nextDutyRequest(t *testing.T, requests <-chan dutyRequest) dutyRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	default:
		t.Fatal("expected a captured FlashDuty request")
		return dutyRequest{}
	}
}

func dutyContext(t *testing.T, tables ...interface{}) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(tables...))
	return ctx.NewContext(context.Background(), db, true)
}

func TestInit_ConfiguresRequestTimeout(t *testing.T) {
	oldAPI, oldHeaders, oldTimeout := Api, Headers, Timeout
	t.Cleanup(func() { Api, Headers, Timeout = oldAPI, oldHeaders, oldTimeout })
	cases := []struct {
		name   string
		millis time.Duration
		want   time.Duration
	}{
		{name: "default", want: 5 * time.Second},
		{name: "configured milliseconds", millis: 125, want: 125 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Init(cconf.FlashDuty{Api: "http://127.0.0.1", Headers: map[string]string{"X-Key": "value"}, Timeout: tc.millis})

			require.Equal(t, tc.want, Timeout)
			require.Equal(t, "http://127.0.0.1", Api)
			require.Equal(t, map[string]string{"X-Key": "value"}, Headers)
		})
	}
}

func TestPostFlashDutyWithResp_BuildsAuthenticatedJSONRequest(t *testing.T) {
	requests := dutyServer(t, map[string]string{"/team/info": `{"data":{"team_id":12,"team_name":"ops"}}`})

	info, err := PostFlashDutyWithResp[TeamInfo]("/team/info", "key &+?", map[string]string{"ref_id": "7"})

	require.NoError(t, err)
	require.Equal(t, int64(12), info.TeamID)
	require.Equal(t, "ops", info.TeamName)
	req := nextDutyRequest(t, requests)
	require.Equal(t, "POST", req.Method)
	require.Equal(t, "/team/info", req.Path)
	require.Equal(t, "key &+?", req.AppKey)
	require.Equal(t, "application/json", req.ContentType)
	require.Equal(t, "secret", req.Auth)
	require.Equal(t, map[string]interface{}{"ref_id": "7"}, req.Body)
}

func TestPostFlashDutyWithResp_ReturnsAPIError(t *testing.T) {
	dutyServer(t, map[string]string{"/member/list": `{"error":{"code":"Denied","message":"permission denied"}}`})

	_, err := PostFlashDutyWithResp[Data]("/member/list", "key", map[string]int{"limit": 100})

	require.EqualError(t, err, "flashduty post error: permission denied")
}

func TestPostJSON_ReturnsResponseStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "unavailable")
	}))
	defer server.Close()

	body, code, err := PostJSON(server.URL, time.Second, nil, map[string]int{"limit": 1}, 1)

	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, code)
	require.Equal(t, "unavailable", string(body))
}

func TestPostJSON_RejectsUnencodableBody(t *testing.T) {
	body, code, err := PostJSON("http://127.0.0.1", time.Second, nil, make(chan int))

	require.ErrorContains(t, err, "unsupported type")
	require.Nil(t, body)
	require.Zero(t, code)
}

func TestPostJSON_RejectsInvalidURL(t *testing.T) {
	for _, retry := range [][]int{nil, {1}} {
		_, code, err := PostJSON("://bad", time.Second, nil, map[string]int{}, retry...)

		require.Error(t, err)
		require.Zero(t, code)
	}
}

func TestMembersAdd_FiltersUsersWithoutReferenceOrContact(t *testing.T) {
	requests := dutyServer(t, nil)
	members := Members{From: "nightingale", Users: []User{
		{RefID: "1", Email: "a@example.test", MemberName: "alice"},
		{RefID: "2", Phone: "123456789"},
		{Email: "missing-id@example.test"},
		{RefID: "3"},
	}}

	require.NoError(t, members.addMembers("key"))

	req := nextDutyRequest(t, requests)
	require.Equal(t, "/member/invite", req.Path)
	require.Equal(t, "nightingale", req.Body["from"])
	users := req.Body["members"].([]interface{})
	require.Len(t, users, 2)
	require.Equal(t, "1", users[0].(map[string]interface{})["ref_id"])
	require.Equal(t, "123456789", users[1].(map[string]interface{})["phone"])
	require.Len(t, members.Users, 2)
}

func TestMembersAdd_SkipsEmptyAndInvalidInvitations(t *testing.T) {
	requests := dutyServer(t, nil)
	cases := []struct {
		name  string
		users []User
	}{
		{name: "empty"},
		{name: "no usable users", users: []User{{RefID: "1"}, {Email: "a@example.test"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			members := Members{Users: tc.users}

			require.NoError(t, members.addMembers("key"))

			require.Empty(t, requests)
		})
	}
}

func TestUserDelete_RequiresReferenceAndSendsForceFlag(t *testing.T) {
	requests := dutyServer(t, nil)

	require.EqualError(t, (&User{}).delMember("key", true), "refID must not be empty")
	require.Empty(t, requests)
	require.NoError(t, (&User{RefID: "42", Email: "private@example.test"}).delMember("key", true))

	req := nextDutyRequest(t, requests)
	require.Equal(t, "/member/delete", req.Path)
	require.Equal(t, "42", req.Body["ref_id"])
	require.Equal(t, true, req.Body["is_force"])
	require.NotContains(t, req.Body, "email")
}

func TestTeamUpdate_SendsUpsertWithResetFlag(t *testing.T) {
	requests := dutyServer(t, nil)
	team := Team{TeamID: 9, TeamName: "ops", RefID: "3", Emails: []string{"a@example.test"}}

	require.NoError(t, team.UpdateTeam("key"))

	req := nextDutyRequest(t, requests)
	require.Equal(t, "/team/upsert", req.Path)
	require.Equal(t, true, req.Body["reset_if_name_exist"])
	require.Equal(t, float64(9), req.Body["team_id"])
	require.Equal(t, "ops", req.Body["team_name"])
	require.Equal(t, []interface{}{"a@example.test"}, req.Body["emails"])
}

func TestTeamAdd_RequiresName(t *testing.T) {
	requests := dutyServer(t, nil)

	err := (&Team{RefID: "3"}).AddTeam("key")

	require.EqualError(t, err, "team_name must be set")
	require.Empty(t, requests)
}

func TestUserGroupSyncer_DeleteUsesGroupReference(t *testing.T) {
	requests := dutyServer(t, nil)
	syncer := UserGroupSyncer{ug: &models.UserGroup{Id: 8}, appKey: "key"}

	require.NoError(t, syncer.SyncUGDel())

	req := nextDutyRequest(t, requests)
	require.Equal(t, "/team/delete", req.Path)
	require.Equal(t, "8", req.Body["ref_id"])
}

func TestUserGroupSyncer_CheckTeamReturnsRemoteID(t *testing.T) {
	requests := dutyServer(t, map[string]string{"/team/info": `{"data":{"team_id":27}}`})
	syncer := UserGroupSyncer{appKey: "key"}

	id, err := syncer.CheckTeam("8")

	require.NoError(t, err)
	require.Equal(t, int64(27), id)
	req := nextDutyRequest(t, requests)
	require.Equal(t, "/team/info", req.Path)
	require.Equal(t, "8", req.Body["ref_id"])
}

func TestUserGroupSyncer_AddMembersPrefersEmailForTeamContacts(t *testing.T) {
	requests := dutyServer(t, map[string]string{"/team/info": `{"data":{"team_id":27}}`})
	syncer := UserGroupSyncer{ug: &models.UserGroup{Id: 8, Name: "ops"}, appKey: "key", syncFrom: "n9e"}
	users := []models.User{
		{Id: 1, Username: "alice", Email: "a@example.test", Phone: "111111111"},
		{Id: 2, Username: "bob", Phone: "222222222"},
		{Id: 3, Username: "no-contact"},
	}

	require.NoError(t, syncer.addMemberToFDTeam(users))

	invite := nextDutyRequest(t, requests)
	require.Equal(t, "/member/invite", invite.Path)
	require.Equal(t, "n9e", invite.Body["from"])
	require.Len(t, invite.Body["members"], 2)
	require.Equal(t, "/team/info", (nextDutyRequest(t, requests)).Path)
	team := nextDutyRequest(t, requests)
	require.Equal(t, "/team/upsert", team.Path)
	require.Equal(t, float64(27), team.Body["team_id"])
	require.Equal(t, []interface{}{"a@example.test"}, team.Body["emails"])
	require.Equal(t, []interface{}{"222222222"}, team.Body["phones"])
}

func TestSyncUsersChange_ReconcilesAddedDeletedAndChangedMembers(t *testing.T) {
	requests := dutyServer(t, map[string]string{
		"/member/list": `{"data":{"total":3,"items":[` +
			`{"ref_id":"1","member_name":"old","email":"old@example.test"},` +
			`{"ref_id":"2","member_name":"removed","email":"gone@example.test"},` +
			`{"ref_id":"","member_name":"unmanaged"}]}}`,
	})
	c := dutyContext(t, &models.Configs{})
	require.NoError(t, c.DB.Create(&[]models.Configs{
		{Ckey: "flashduty_app_key", Cval: "key"},
		{Ckey: "flashduty_force_delete_user", Cval: "true"},
		{Ckey: "flashduty_sync_user_from", Cval: "n9e"},
	}).Error)
	users := []*models.User{
		{Id: 1, Username: "alice", Email: "a@example.test"},
		{Id: 3, Username: "bob", Phone: "123456789"},
	}

	require.NoError(t, SyncUsersChange(c, users))

	require.Equal(t, "/member/list", (nextDutyRequest(t, requests)).Path)
	page := nextDutyRequest(t, requests)
	require.Equal(t, float64(0), page.Body["p"])
	require.Equal(t, float64(100), page.Body["limit"])
	deleted := nextDutyRequest(t, requests)
	require.Equal(t, "/member/delete", deleted.Path)
	require.Equal(t, "2", deleted.Body["ref_id"])
	require.Equal(t, true, deleted.Body["is_force"])
	added := nextDutyRequest(t, requests)
	require.Equal(t, "/member/invite", added.Path)
	require.Equal(t, "3", added.Body["members"].([]interface{})[0].(map[string]interface{})["ref_id"])
	changed := nextDutyRequest(t, requests)
	require.Equal(t, "/member/info/reset", changed.Path)
	require.Equal(t, "1", changed.Body["ref_id"])
	require.Equal(t, "a@example.test", changed.Body["updates"].(map[string]interface{})["email"])
	require.Empty(t, requests)
}

func TestUpdateUser_InvitesMissingRemoteMember(t *testing.T) {
	requests := dutyServer(t, map[string]string{
		"/member/info/reset": `{"error":{"message":"no member found"}}`,
	})
	c := dutyContext(t, &models.Configs{})
	require.NoError(t, c.DB.Create(&models.Configs{Ckey: "flashduty_app_key", Cval: "key"}).Error)

	UpdateUser(c, models.User{Id: 5, Username: "alice"}, "a@example.test", "123456789")

	update := nextDutyRequest(t, requests)
	require.Equal(t, "/member/info/reset", update.Path)
	require.Equal(t, "5", update.Body["ref_id"])
	invite := nextDutyRequest(t, requests)
	require.Equal(t, "/member/invite", invite.Path)
	member := invite.Body["members"].([]interface{})[0].(map[string]interface{})
	require.Equal(t, "alice", member["member_name"])
	require.Equal(t, "a@example.test", member["email"])
	require.Equal(t, "123456789", member["phone"])
}

func TestSyncUsersChange_EdgeSkipsSync(t *testing.T) {
	requests := dutyServer(t, nil)

	err := SyncUsersChange(&ctx.Context{IsCenter: false}, []*models.User{{Id: 1}})

	require.NoError(t, err)
	require.Empty(t, requests)
}

func TestFlashDutyConfig_ReadsPersistedSyncOptions(t *testing.T) {
	c := dutyContext(t, &models.Configs{})
	require.False(t, NeedSyncUser(c))
	require.False(t, NeedSyncTeam(c))
	require.False(t, NeedForceDeleteUser(c))
	require.Empty(t, GetFlashDutySyncUserFrom(c))
	require.NoError(t, c.DB.Create(&[]models.Configs{
		{Ckey: "flashduty_app_key", Cval: "key"},
		{Ckey: "flashduty_sync_team", Cval: "true"},
		{Ckey: "flashduty_force_delete_user", Cval: "true"},
		{Ckey: "flashduty_sync_user_from", Cval: "n9e"},
	}).Error)

	require.True(t, NeedSyncUser(c))
	require.True(t, NeedSyncTeam(c))
	require.True(t, NeedForceDeleteUser(c))
	require.Equal(t, "n9e", GetFlashDutySyncUserFrom(c))
	syncer, err := NewUserGroupSyncer(c, &models.UserGroup{Id: 7})
	require.NoError(t, err)
	require.Equal(t, "key", syncer.appKey)
	require.Equal(t, "n9e", syncer.syncFrom)
}
