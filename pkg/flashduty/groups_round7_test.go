package flashduty

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func groupSyncerContext(t *testing.T) *UserGroupSyncer {
	t.Helper()
	c := dutyContext(t, &models.User{}, &models.UserGroupMember{})
	users := []models.User{
		{Id: 1, Username: "email-user", Email: "alice@example.test", Phone: "111"},
		{Id: 2, Username: "phone-user", Phone: "222"},
		{Id: 3, Username: "no-contact"},
	}
	require.NoError(t, c.DB.Create(&users).Error)
	members := []models.UserGroupMember{{GroupId: 7, UserId: 1}, {GroupId: 7, UserId: 2}, {GroupId: 7, UserId: 3}}
	require.NoError(t, c.DB.Create(&members).Error)
	return &UserGroupSyncer{
		ctx: c, ug: &models.UserGroup{Id: 7, Name: "SRE", Users: users}, appKey: "key", syncFrom: "n9e",
	}
}

func TestSyncUGAdd_CreatesTeamThenSynchronizesPersistedMembers(t *testing.T) {
	requests := dutyServer(t, map[string]string{"/team/info": `{"data":{"team_id":42}}`})
	syncer := groupSyncerContext(t)

	err := syncer.SyncUGAdd()

	require.NoError(t, err)
	created := nextDutyRequest(t, requests)
	require.Equal(t, "/team/upsert", created.Path)
	require.Equal(t, "7", created.Body["ref_id"])
	require.Equal(t, "SRE", created.Body["team_name"])
	invited := nextDutyRequest(t, requests)
	require.Equal(t, "/member/invite", invited.Path)
	require.Equal(t, "n9e", invited.Body["from"])
	require.Len(t, invited.Body["members"], 2)
	lookup := nextDutyRequest(t, requests)
	require.Equal(t, "/team/info", lookup.Path)
	updated := nextDutyRequest(t, requests)
	require.Equal(t, "/team/upsert", updated.Path)
	require.Equal(t, float64(42), updated.Body["team_id"])
	require.Equal(t, []interface{}{"alice@example.test"}, updated.Body["emails"])
	require.Equal(t, []interface{}{"222"}, updated.Body["phones"])
}

func TestSyncUGPut_UpdatesRemoteTeamAndRefreshesMembers(t *testing.T) {
	requests := dutyServer(t, map[string]string{"/team/info": `{"data":{"team_id":42}}`})
	syncer := groupSyncerContext(t)

	err := syncer.SyncUGPut()

	require.NoError(t, err)
	require.Equal(t, "/team/info", nextDutyRequest(t, requests).Path)
	updated := nextDutyRequest(t, requests)
	require.Equal(t, "/team/upsert", updated.Path)
	require.Equal(t, float64(42), updated.Body["team_id"])
	require.Equal(t, true, updated.Body["reset_if_name_exist"])
	require.Equal(t, []interface{}{"alice@example.test"}, updated.Body["emails"])
	require.Equal(t, []interface{}{"222"}, updated.Body["phones"])
	require.Equal(t, "/member/invite", nextDutyRequest(t, requests).Path)
	refreshed := nextDutyRequest(t, requests)
	require.Equal(t, "/team/upsert", refreshed.Path)
	require.Equal(t, float64(42), refreshed.Body["team_id"])
	require.Equal(t, "7", refreshed.Body["ref_id"])
}

func TestSyncUGPut_CreatesTeamWhenReferenceIsMissing(t *testing.T) {
	requests := make(chan dutyRequest, 8)
	infoCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- dutyRequest{Path: r.URL.Path, Body: body}
		response := `{"data":{}}`
		if r.URL.Path == "/team/info" {
			infoCalls++
			response = `{"error":{"message":"no team found by ref_id"}}`
			if infoCalls > 1 {
				response = `{"data":{"team_id":42}}`
			}
		}
		_, _ = w.Write([]byte(response))
	}))
	oldAPI := Api
	Api = server.URL
	t.Cleanup(func() { server.Close(); Api = oldAPI })
	syncer := groupSyncerContext(t)

	err := syncer.SyncUGPut()

	require.NoError(t, err)
	require.Equal(t, "/team/info", nextDutyRequest(t, requests).Path)
	created := nextDutyRequest(t, requests)
	require.Equal(t, "/team/upsert", created.Path)
	require.Equal(t, false, created.Body["reset_if_name_exist"])
	require.Equal(t, "7", created.Body["ref_id"])
	require.Equal(t, []interface{}{"alice@example.test"}, created.Body["emails"])
	require.Equal(t, []interface{}{"222"}, created.Body["phones"])
	require.Equal(t, "/member/invite", nextDutyRequest(t, requests).Path)
	require.Equal(t, "/team/info", nextDutyRequest(t, requests).Path)
	require.Equal(t, "/team/upsert", nextDutyRequest(t, requests).Path)
}

func TestSyncUGPut_PropagatesLookupFailure(t *testing.T) {
	requests := dutyServer(t, map[string]string{"/team/info": `{"error":{"message":"permission denied"}}`})
	syncer := groupSyncerContext(t)

	err := syncer.SyncUGPut()

	require.ErrorContains(t, err, "permission denied")
	require.Equal(t, "/team/info", nextDutyRequest(t, requests).Path)
	require.Empty(t, requests)
}

func TestSyncUGAdd_StopsWhenTeamCreationFails(t *testing.T) {
	requests := dutyServer(t, map[string]string{"/team/upsert": `{"error":{"message":"team rejected"}}`})
	syncer := groupSyncerContext(t)

	err := syncer.SyncUGAdd()

	require.ErrorContains(t, err, "team rejected")
	require.Equal(t, "/team/upsert", nextDutyRequest(t, requests).Path)
	require.Empty(t, requests)
}

func TestSyncMembersAdd_ReportsMembershipQueryFailure(t *testing.T) {
	requests := dutyServer(t, nil)
	syncer := groupSyncerContext(t)
	require.NoError(t, syncer.ctx.DB.Migrator().DropTable(&models.UserGroupMember{}))

	err := syncer.SyncMembersAdd()

	require.Error(t, err)
	require.Empty(t, requests)
}

func TestSyncMembersDel_ReportsUserQueryFailure(t *testing.T) {
	requests := dutyServer(t, nil)
	syncer := groupSyncerContext(t)
	require.NoError(t, syncer.ctx.DB.Migrator().DropTable(&models.User{}))

	err := syncer.SyncMembersDel()

	require.Error(t, err)
	require.Empty(t, requests)
}
