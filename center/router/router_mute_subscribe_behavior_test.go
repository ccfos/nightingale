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
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func muteSubscribeRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func seedMuteSubscribeMembership(t *testing.T, rt *Router) {
	t.Helper()
	db := rt.Ctx.DB
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "production"}).Error)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 2, Name: "staging"}).Error)
	require.NoError(t, db.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: "rw"}).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
}

func muteSubscribeRequest(t *testing.T, handler gin.HandlerFunc, body interface{}, path string,
	user *models.User) *httptest.ResponseRecorder {
	t.Helper()
	// JSONArr implements MarshalJSON on its pointer; marshal addressable models to match HTTP JSON arrays.
	switch v := body.(type) {
	case models.AlertMute:
		body = &v
	case models.AlertSubscribe:
		body = &v
	case SubscribeTryRunForm:
		body = &v
	}
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("username", "editor"); c.Set("user", user) })
	engine.POST("/groups/:id/objects/:amid/:sid", handler)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Language", "en_US")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func muteSubscribeReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
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

func TestAlertMuteAdd_UsesAuthenticatedAuthorAndRouteGroup(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertMute{},
	)
	mute := models.AlertMute{
		GroupId: 2, Cause: "maintenance", Btime: 100, Etime: 200, CreateBy: "spoofed",
		DatasourceIdsJson: []int64{3}, SeveritiesJson: []int{2}, Tags: ormx.JSONArr(`[]`),
	}

	w := muteSubscribeRequest(t, rt.alertMuteAdd, mute, "/groups/1/objects/0/0", nil)

	var id int64
	require.Empty(t, muteSubscribeReply(t, w, &id))
	var saved models.AlertMute
	require.NoError(t, rt.Ctx.DB.First(&saved, id).Error)
	require.Equal(t, int64(1), saved.GroupId)
	require.Equal(t, "editor", saved.CreateBy)
	require.Equal(t, "editor", saved.UpdateBy)
	require.Equal(t, "maintenance", saved.Cause)
	require.JSONEq(t, `[3]`, saved.DatasourceIds)
	require.JSONEq(t, `[2]`, saved.Severities)
}

func TestAlertMuteAdd_RejectsInvalidTimeRangeWithoutInsert(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertMute{},
	)

	w := muteSubscribeRequest(t, rt.alertMuteAdd, models.AlertMute{Btime: 200, Etime: 100}, "/groups/1/objects/0/0", nil)

	require.Contains(t, muteSubscribeReply(t, w, nil), "etime(100) <= btime(200)")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AlertMute{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAlertMutePut_EnforcesStoredGroupOwnership(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(fmt.Sprintf("allowed=%t", allowed), func(t *testing.T) {
			rt := muteSubscribeRouter(t,
				&models.AlertMute{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedMuteSubscribeMembership(t, rt)
			original := models.AlertMute{Id: 1, GroupId: 1, Btime: 100, Etime: 200, Cause: "original", CreateBy: "creator"}
			require.NoError(t, original.FE2DB())
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			user := &models.User{Id: 1}
			if !allowed {
				user.Id = 2
			}
			replacement := models.AlertMute{GroupId: 2, Btime: 300, Etime: 400, Cause: "updated", CreateBy: "spoofed"}

			w := muteSubscribeRequest(t, rt.alertMutePutByFE, replacement, "/groups/2/objects/1/0", user)

			var saved models.AlertMute
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, original.GroupId, saved.GroupId)
			require.Equal(t, original.CreateBy, saved.CreateBy)
			if allowed {
				require.Empty(t, muteSubscribeReply(t, w, nil))
				require.Equal(t, replacement.Cause, saved.Cause)
				require.Equal(t, replacement.Etime, saved.Etime)
				require.Equal(t, "editor", saved.UpdateBy)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Equal(t, original.Cause, saved.Cause)
				require.Equal(t, original.Etime, saved.Etime)
			}
		})
	}
}

func TestAlertMutePut_ReturnsNotFound(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertMute{},
	)

	w := muteSubscribeRequest(t, rt.alertMutePutByFE, models.AlertMute{}, "/groups/1/objects/404/0", &models.User{Id: 1})

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Equal(t, "No such AlertMute", w.Body.String())
}

func TestAlertMuteGet_ReturnsNotFound(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertMute{},
	)

	w := muteSubscribeRequest(t, rt.alertMuteGet, nil, "/groups/1/objects/404/0", &models.User{Id: 1})

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Equal(t, "No such AlertMute", w.Body.String())
}

func TestAlertMuteGets_FiltersGroupProductDisabledAndCause(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertMute{},
		&models.User{},
	)
	mutes := []models.AlertMute{
		{Id: 1, GroupId: 1, Prod: "metric", Cause: "planned database maintenance", Tags: ormx.JSONArr(`[]`)},
		{Id: 2, GroupId: 1, Prod: "metric", Cause: "planned database maintenance", Disabled: 1},
		{Id: 3, GroupId: 2, Prod: "metric", Cause: "planned database maintenance"},
		{Id: 4, GroupId: 1, Prod: "host", Cause: "planned database maintenance"},
		{Id: 5, GroupId: 1, Prod: "metric", Cause: "planned web maintenance"},
	}
	require.NoError(t, rt.Ctx.DB.Create(&mutes).Error)

	w := muteSubscribeRequest(t, rt.alertMuteGets, nil,
		"/groups/1/objects/0/0?bgid=1&prods=metric&disabled=0&query=planned%20database", nil)

	var got []models.AlertMute
	require.Empty(t, muteSubscribeReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
}

func TestAlertMuteGetsByGids_RestrictsImplicitAndExplicitGroups(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		query  string
		status int
		ids    []int64
	}{
		{name: "member implicit", user: models.User{Id: 1}, status: http.StatusOK, ids: []int64{1}},
		{name: "outsider implicit", user: models.User{Id: 2}, status: http.StatusOK, ids: []int64{}},
		{name: "member explicit", user: models.User{Id: 1}, query: "?gids=1", status: http.StatusOK, ids: []int64{1}},
		{name: "foreign explicit", user: models.User{Id: 1}, query: "?gids=2", status: http.StatusForbidden},
		{
			name: "admin implicit",
			user: models.User{
				RolesLst: []string{
					models.AdminRole,
				},
			},
			status: http.StatusOK,
			ids: []int64{
				1,
				2,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := muteSubscribeRouter(t,
				&models.AlertMute{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
				&models.User{},
			)
			seedMuteSubscribeMembership(t, rt)
			require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertMute{{Id: 1, GroupId: 1}, {Id: 2, GroupId: 2}}).Error)

			w := muteSubscribeRequest(t, rt.alertMuteGetsByGids, nil, "/groups/1/objects/0/0"+tc.query, &tc.user)

			require.Equal(t, tc.status, w.Code)
			if tc.status != http.StatusOK {
				return
			}
			var got []models.AlertMute
			require.Empty(t, muteSubscribeReply(t, w, &got))
			ids := make([]int64, 0, len(got))
			for _, mute := range got {
				ids = append(ids, mute.Id)
			}
			require.ElementsMatch(t, tc.ids, ids)
		})
	}
}

func TestAlertMutePutFields_UpdatesExistingRowsAndSkipsMissingIDs(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertMute{},
	)
	mute := models.AlertMute{Id: 1, Cause: "keep", UpdateBy: "creator", UpdateAt: 1}
	require.NoError(t, mute.FE2DB())
	require.NoError(t, rt.Ctx.DB.Create(&mute).Error)
	form := alertMuteFieldForm{Ids: []int64{404, 1}, Fields: map[string]interface{}{"disabled": 1}}

	w := muteSubscribeRequest(t, rt.alertMutePutFields, form, "/groups/1/objects/0/0", nil)

	require.Empty(t, muteSubscribeReply(t, w, nil))
	var saved models.AlertMute
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, 1, saved.Disabled)
	require.Equal(t, "keep", saved.Cause)
	require.Equal(t, "editor", saved.UpdateBy)
	require.NotEqual(t, int64(1), saved.UpdateAt)
}

func TestAlertMutePreview_FiltersEventsByTags(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertCurEvent{},
	)
	events := []models.AlertCurEvent{
		{Id: 1, GroupId: 1, RuleProd: "host", Severity: 2, Tags: "env=prod,,region=us"},
		{Id: 2, GroupId: 1, RuleProd: "host", Severity: 2, Tags: "env=stage,,region=us"},
		{Id: 3, GroupId: 2, RuleProd: "host", Severity: 2, Tags: "env=prod,,region=us"},
	}
	require.NoError(t, rt.Ctx.DB.Create(&events).Error)
	mute := models.AlertMute{
		Prod: "host", Btime: 100, Etime: 200, SeveritiesJson: []int{2},
		Tags: ormx.JSONArr(`[{"key":"env","func":"==","value":"prod"}]`),
	}

	w := muteSubscribeRequest(t, rt.alertMutePreview, mute, "/groups/1/objects/0/0", nil)

	var got []models.AlertCurEvent
	require.Empty(t, muteSubscribeReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
	require.Equal(t, "prod", got[0].TagsMap["env"])
}

func TestAlertSubscribeAdd_UsesRouteGroupAndSerializesMatchingOptions(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertSubscribe{},
	)
	sub := models.AlertSubscribe{
		Name: "critical", GroupId: 2, SeveritiesJson: []int{1, 2}, DatasourceIdsJson: []int64{3},
		NotifyVersion: 1, NotifyRuleIds: []int64{9}, CreateBy: "spoofed",
	}

	w := muteSubscribeRequest(t, rt.alertSubscribeAdd, sub, "/groups/1/objects/0/0", nil)

	require.Empty(t, muteSubscribeReply(t, w, nil))
	var saved models.AlertSubscribe
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, int64(1), saved.GroupId)
	require.Equal(t, "editor", saved.CreateBy)
	require.Equal(t, "editor", saved.UpdateBy)
	require.JSONEq(t, `[1,2]`, saved.Severities)
	require.JSONEq(t, `[3]`, saved.DatasourceIds)
	require.Equal(t, []int64{9}, saved.NotifyRuleIds)
}

func TestAlertSubscribeAdd_RejectsInvalidMatchingAndNotificationSettings(t *testing.T) {
	cases := []struct {
		name    string
		group   string
		sub     models.AlertSubscribe
		message string
	}{
		{name: "invalid group", group: "0", sub: models.AlertSubscribe{}, message: "group_id invalid"},
		{name: "missing severities", group: "1", sub: models.AlertSubscribe{}, message: "severities is required"},
		{
			name: "no notify rules", group: "1", sub: models.AlertSubscribe{SeveritiesJson: []int{2}, NotifyVersion: 1},
			message: "no notify rules selected",
		},
		{
			name: "no channels", group: "1", sub: models.AlertSubscribe{SeveritiesJson: []int{2}, UserGroupIds: "10"},
			message: "new_channels is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := muteSubscribeRouter(t,
				&models.AlertSubscribe{},
			)

			w := muteSubscribeRequest(t, rt.alertSubscribeAdd, tc.sub, "/groups/"+tc.group+"/objects/0/0", nil)

			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.AlertSubscribe{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestAlertSubscribePut_ReplacesLegacyRuleIDWithoutMovingGroup(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertSubscribe{},
	)
	original := models.AlertSubscribe{Id: 1, Name: "original", GroupId: 1, RuleId: 5, CreateBy: "creator"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	changed := models.AlertSubscribe{
		Id: 1, Name: "updated", GroupId: 2, RuleId: 99, RuleIds: []int64{6, 7}, SeveritiesJson: []int{2},
	}

	w := muteSubscribeRequest(t, rt.alertSubscribePut, []models.AlertSubscribe{changed}, "/groups/1/objects/0/0", nil)

	require.Empty(t, muteSubscribeReply(t, w, nil))
	var saved models.AlertSubscribe
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Zero(t, saved.RuleId)
	require.Equal(t, changed.RuleIds, saved.RuleIds)
	require.Equal(t, original.GroupId, saved.GroupId)
	require.Equal(t, original.CreateBy, saved.CreateBy)
	require.Equal(t, "editor", saved.UpdateBy)
	require.Equal(t, changed.Name, saved.Name)
}

func TestAlertSubscribeGet_FillsRuleAndTeamNames(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertSubscribe{},
		&models.UserGroup{},
		&models.AlertRule{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 10, Name: "oncall"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertRule{Id: 5, Name: "cpu"}).Error)
	sub := models.AlertSubscribe{
		Id: 1, Name: "subscription", GroupId: 1, RuleIds: []int64{5}, UserGroupIds: "10", Severities: "[2]",
	}
	require.NoError(t, rt.Ctx.DB.Create(&sub).Error)

	w := muteSubscribeRequest(t, rt.alertSubscribeGet, nil, "/groups/1/objects/0/1", nil)

	var got models.AlertSubscribe
	require.Empty(t, muteSubscribeReply(t, w, &got))
	require.Equal(t, []string{"cpu"}, got.RuleNames)
	require.Len(t, got.UserGroups, 1)
	require.Equal(t, "oncall", got.UserGroups[0].Name)
	require.Equal(t, []int{2}, got.SeveritiesJson)
}

func TestAlertSubscribeGet_ReturnsNotFound(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertSubscribe{},
	)

	w := muteSubscribeRequest(t, rt.alertSubscribeGet, nil, "/groups/1/objects/0/404", nil)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Equal(t, "No such alert subscribe", w.Body.String())
}

func TestAlertSubscribeTryRun_ReportsNonmatchingEventCriteria(t *testing.T) {
	cases := []struct {
		name    string
		change  func(*models.AlertSubscribe)
		message string
	}{
		{
			name: "datasource",
			change: func(s *models.AlertSubscribe) {
				s.DatasourceIdsJson = []int64{
					9,
				}
			},
			message: "datasource not match",
		},
		{name: "rule", change: func(s *models.AlertSubscribe) { s.RuleIds = []int64{9} }, message: "rule id not match"},
		{
			name: "tags", change: func(s *models.AlertSubscribe) {
				s.Tags = ormx.JSONArr(`[{"key":"env","func":"==","value":"stage"}]`)
			}, message: "tags not match",
		},
		{
			name: "severity",
			change: func(s *models.AlertSubscribe) {
				s.SeveritiesJson = []int{
					1,
				}
			},
			message: "severity not match",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := muteSubscribeRouter(t,
				&models.AlertHisEvent{},
			)
			event := models.AlertHisEvent{
				Id:           1,
				RuleId:       5,
				DatasourceId: 3,
				Severity:     2,
				Tags:         "env=prod",
				GroupName:    "production",
			}
			require.NoError(t, rt.Ctx.DB.Create(&event).Error)
			sub := models.AlertSubscribe{SeveritiesJson: []int{2}, DatasourceIdsJson: []int64{3}}
			tc.change(&sub)

			w := muteSubscribeRequest(t, rt.alertSubscribeTryRun,
				SubscribeTryRunForm{EventId: 1, SubscribeConfig: sub}, "/groups/1/objects/0/0", nil)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
		})
	}
}

func TestAlertSubscribeGetsByGids_RestrictsMembershipAndFillsRules(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		query  string
		status int
		ids    []int64
	}{
		{name: "implicit membership", user: models.User{Id: 1}, status: http.StatusOK, ids: []int64{1}},
		{name: "no membership", user: models.User{Id: 2}, status: http.StatusOK, ids: []int64{}},
		{name: "explicit own", user: models.User{Id: 1}, query: "?gids=1", status: http.StatusOK, ids: []int64{1}},
		{name: "explicit foreign", user: models.User{Id: 1}, query: "?gids=2", status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := muteSubscribeRouter(t,
				&models.AlertSubscribe{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
				&models.User{},
				&models.AlertRule{},
			)
			seedMuteSubscribeMembership(t, rt)
			require.NoError(t, rt.Ctx.DB.Create(&models.AlertRule{Id: 5, Name: "cpu"}).Error)
			subs := []models.AlertSubscribe{
				{Id: 1, GroupId: 1, RuleIds: []int64{5}, Severities: "[2]"}, {Id: 2, GroupId: 2, Severities: "[2]"},
			}
			require.NoError(t, rt.Ctx.DB.Create(&subs).Error)

			w := muteSubscribeRequest(t, rt.alertSubscribeGetsByGids, nil, "/groups/1/objects/0/0"+tc.query, &tc.user)

			require.Equal(t, tc.status, w.Code)
			if tc.status != http.StatusOK {
				return
			}
			var got []models.AlertSubscribe
			require.Empty(t, muteSubscribeReply(t, w, &got))
			ids := make([]int64, 0, len(got))
			for _, sub := range got {
				ids = append(ids, sub.Id)
				require.Equal(t, []string{"cpu"}, sub.RuleNames)
				require.Equal(t, []int{2}, sub.SeveritiesJson)
			}
			require.Equal(t, tc.ids, ids)
		})
	}
}

func TestAlertSubscribeGets_OnlyReturnsRouteGroup(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertSubscribe{},
		&models.User{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertSubscribe{
		{Id: 1, GroupId: 1, Name: "own", Severities: "[2]"}, {Id: 2, GroupId: 2, Name: "other", Severities: "[1]"},
	}).Error)

	w := muteSubscribeRequest(t, rt.alertSubscribeGets, nil, "/groups/1/objects/0/0", nil)

	var got []models.AlertSubscribe
	require.Empty(t, muteSubscribeReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "own", got[0].Name)
	require.Equal(t, []int{2}, got[0].SeveritiesJson)
}

func TestAlertSubscribeTryRun_ReportsMissingNotificationSettingsAfterMatch(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertHisEvent{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{
		Id: 1, RuleId: 5, DatasourceId: 3, Severity: 2, Tags: "env=prod", GroupName: "production",
	}).Error)
	sub := models.AlertSubscribe{
		RuleIds: []int64{5}, DatasourceIdsJson: []int64{3}, SeveritiesJson: []int{2},
		Tags: ormx.JSONArr(`[{"key":"env","func":"==","value":"prod"}]`),
	}

	w := muteSubscribeRequest(t, rt.alertSubscribeTryRun,
		SubscribeTryRunForm{EventId: 1, SubscribeConfig: sub}, "/groups/1/objects/0/0", nil)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "no notify channels selected")
}

func TestAlertMuteTryRun_CanBypassTimeCheckForHistoricalEvents(t *testing.T) {
	rt := muteSubscribeRouter(t,
		&models.AlertHisEvent{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{
		Id: 1, TriggerTime: 100, Severity: 2, Tags: "env=prod",
	}).Error)
	form := MuteTestForm{
		EventId: 1, PassTimeCheck: true,
		AlertMute: models.AlertMute{
			Btime: 200, Etime: 300, SeveritiesJson: []int{2},
			Tags: ormx.JSONArr(`[{"key":"env","func":"==","value":"prod"}]`),
		},
	}

	w := muteSubscribeRequest(t, rt.alertMuteTryRun, &form, "/groups/1/objects/0/0", nil)

	var result string
	require.Empty(t, muteSubscribeReply(t, w, &result))
	require.Equal(t, "event match mute", result)
}
