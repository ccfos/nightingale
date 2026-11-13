package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func eventListingRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	rt := &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
	rt.Center.EventHistoryGroupView = true
	return rt
}

func seedEventListingMembership(t *testing.T, rt *Router) {
	t.Helper()
	db := rt.Ctx.DB
	require.NoError(t, db.Create(&[]models.BusiGroup{{Id: 1, Name: "production"}, {Id: 2, Name: "restricted"}}).Error)
	require.NoError(t, db.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: "rw"}).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
}

func eventListingRequest(t *testing.T, handler gin.HandlerFunc, body interface{}, query string,
	user *models.User) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("user", user) })
	engine.POST("/events/:eid", handler)
	req := httptest.NewRequest(http.MethodPost, "/events/1"+query, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func eventListingReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
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

func TestGetBusinessGroupIds_EnforcesConfiguredVisibility(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		user     models.User
		myGroups bool
		expected []int64
		denied   bool
	}{
		{name: "own groups", path: "/api/events", user: models.User{Id: 1}, expected: []int64{1}},
		{name: "no memberships", path: "/api/events", user: models.User{Id: 2}, expected: []int64{0}},
		{name: "selected own", path: "/api/events?bgid=1", user: models.User{Id: 1}, expected: []int64{1}},
		{name: "selected foreign", path: "/api/events?bgid=2", user: models.User{Id: 1}, denied: true},
		{name: "admin all", path: "/api/events", user: models.User{RolesLst: []string{models.AdminRole}}},
		{name: "service filter", path: "/v1/events?bgid=2", expected: []int64{2}},
		{name: "service all", path: "/v1/events"},
		{name: "admin my groups", path: "/api/events", user: models.User{Id: 1, RolesLst: []string{models.AdminRole}},
			myGroups: true, expected: []int64{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := eventListingRouter(t,
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedEventListingMembership(t, rt)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
			c.Set("user", &tc.user)

			got, err := GetBusinessGroupIds(c, rt.Ctx, true, tc.myGroups)

			if tc.denied {
				require.EqualError(t, err, "business group ID not allowed")
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, got)
			}
		})
	}
}

func TestAlertCurEventsList_AppliesFiltersAndPagination(t *testing.T) {
	rt := eventListingRouter(t,
		&models.AlertCurEvent{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedEventListingMembership(t, rt)
	events := []models.AlertCurEvent{
		{Id: 1, GroupId: 1, RuleProd: "metric", Cate: "prometheus", Severity: 2, RuleId: 5, TriggerTime: 100},
		{Id: 2, GroupId: 1, RuleProd: "metric", Cate: "prometheus", Severity: 2, RuleId: 5, TriggerTime: 200},
		{Id: 3, GroupId: 2, RuleProd: "metric", Cate: "prometheus", Severity: 2, RuleId: 5, TriggerTime: 300},
		{Id: 4, GroupId: 1, RuleProd: "metric", Cate: "prometheus", Severity: 1, RuleId: 5, TriggerTime: 400},
	}
	require.NoError(t, rt.Ctx.DB.Create(&events).Error)

	w := eventListingRequest(t, rt.alertCurEventsList, nil,
		"?rule_prods=metric&cate=prometheus&severity=2&rid=5&stime=50&etime=500&limit=1&p=2", &models.User{Id: 1})

	var got struct {
		List  []models.AlertCurEvent `json:"list"`
		Total int                    `json:"total"`
	}
	require.Empty(t, eventListingReply(t, w, &got))
	require.Equal(t, 2, got.Total)
	require.Len(t, got.List, 1)
	require.Equal(t, int64(1), got.List[0].Id)
}

func TestAlertCurEventsList_FiltersByPostedEventIDs(t *testing.T) {
	rt := eventListingRouter(t,
		&models.AlertCurEvent{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedEventListingMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertCurEvent{{Id: 1, GroupId: 1}, {Id: 2, GroupId: 1}}).Error)

	w := eventListingRequest(t, rt.alertCurEventsList, json.RawMessage(`{"event_ids":[2]}`), "", &models.User{Id: 1})

	var got struct {
		List  []models.AlertCurEvent `json:"list"`
		Total int                    `json:"total"`
	}
	require.Empty(t, eventListingReply(t, w, &got))
	require.Equal(t, 1, got.Total)
	require.Len(t, got.List, 1)
	require.Equal(t, int64(2), got.List[0].Id)
}

func TestAlertCurEventDelete_ChecksAllGroupsBeforeDeleting(t *testing.T) {
	rt := eventListingRouter(t,
		&models.AlertCurEvent{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedEventListingMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertCurEvent{{Id: 1, GroupId: 1}, {Id: 2, GroupId: 2}}).Error)

	w := eventListingRequest(t, rt.alertCurEventDel, idsForm{Ids: []int64{1, 2}}, "", &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AlertCurEvent{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestAlertCurEventDelete_AllowsWritableGroupsAndSkipsMissingIDs(t *testing.T) {
	rt := eventListingRouter(t,
		&models.AlertCurEvent{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedEventListingMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertCurEvent{{Id: 1, GroupId: 1}, {Id: 2, GroupId: 1}}).Error)

	w := eventListingRequest(t, rt.alertCurEventDel, idsForm{Ids: []int64{1, 2, 404}}, "", &models.User{Id: 1})

	require.Empty(t, eventListingReply(t, w, nil))
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AlertCurEvent{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAlertCurEventsCard_AggregatesAndSortsBySeverityThenCount(t *testing.T) {
	rt := eventListingRouter(t,
		&models.AlertCurEvent{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.AlertAggrView{},
	)
	seedEventListingMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertAggrView{Id: 1, Name: "by service", Rule: "tagkey:service"}).Error)
	events := []models.AlertCurEvent{
		{Id: 1, GroupId: 1, Severity: 2, Tags: "service=web"},
		{Id: 2, GroupId: 1, Severity: 1, Tags: "service=web"},
		{Id: 3, GroupId: 1, Severity: 1, Tags: "service=db"},
		{Id: 4, GroupId: 1, Severity: 3, Tags: "service=cache"},
	}
	require.NoError(t, rt.Ctx.DB.Create(&events).Error)

	w := eventListingRequest(t, rt.alertCurEventsCard, nil, "?view_id=1", &models.User{Id: 1})

	var cards []AlertCard
	require.Empty(t, eventListingReply(t, w, &cards))
	require.Len(t, cards, 3)
	require.Equal(t, "web", cards[0].Title)
	require.Equal(t, 1, cards[0].Severity)
	require.Equal(t, 2, cards[0].Total)
	require.ElementsMatch(t, []int64{1, 2}, cards[0].EventIds)
	require.Equal(t, "db", cards[1].Title)
	require.Equal(t, "cache", cards[2].Title)
}

func TestEventTagValues_RanksValuesByFrequencyWithinVisibleGroups(t *testing.T) {
	rt := eventListingRouter(t,
		&models.AlertCurEvent{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedEventListingMembership(t, rt)
	// The handler always uses the last day; stay far from either boundary.
	trigger := time.Now().Add(-time.Hour).Unix()
	events := []models.AlertCurEvent{
		{GroupId: 1, TriggerTime: trigger, RuleConfig: "{}", Annotations: "{}", Tags: "env=prod"},
		{GroupId: 1, TriggerTime: trigger, RuleConfig: "{}", Annotations: "{}", Tags: "env=prod"},
		{GroupId: 1, TriggerTime: trigger, RuleConfig: "{}", Annotations: "{}", Tags: "env=stage"},
		{GroupId: 2, TriggerTime: trigger, RuleConfig: "{}", Annotations: "{}", Tags: "env=foreign"},
	}
	require.NoError(t, rt.Ctx.DB.Create(&events).Error)

	w := eventListingRequest(t, rt.eventTagValues, nil, "?key=env", &models.User{Id: 1})

	var got []string
	require.Empty(t, eventListingReply(t, w, &got))
	require.Equal(t, []string{"prod", "stage"}, got)
}

func TestEventTagKeys_DeduplicatesVisibleEventTags(t *testing.T) {
	rt := eventListingRouter(t,
		&models.AlertCurEvent{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedEventListingMembership(t, rt)
	trigger := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertCurEvent{
		{GroupId: 1, TriggerTime: trigger, RuleConfig: "{}", Annotations: "{}", Tags: "env=prod,,region=us"},
		{GroupId: 1, TriggerTime: trigger, RuleConfig: "{}", Annotations: "{}", Tags: "env=stage,,service=api"},
	}).Error)

	w := eventListingRequest(t, rt.eventTagKeys, nil, "", &models.User{Id: 1})

	var got []string
	require.Empty(t, eventListingReply(t, w, &got))
	require.ElementsMatch(t, []string{"env", "region", "service"}, got)
}

func TestGetCurEventDetail_FillsNotificationVersionAndNames(t *testing.T) {
	rt := eventListingRouter(t,
		&models.AlertCurEvent{},
		&models.AlertRule{},
		&models.NotifyRule{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.NotifyRule{ID: 9, Name: "oncall"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertCurEvent{
		Id:     1,
		RuleId: 5,
		NotifyRuleIds: []int64{
			9,
		},
		TriggerTime: 123,
	}).Error)

	event, err := GetCurEventDetail(rt.Ctx, 1)

	require.NoError(t, err)
	require.Equal(t, int64(123), event.LastEvalTime)
	require.Equal(t, 1, event.NotifyVersion)
	require.Len(t, event.NotifyRules, 1)
	require.Equal(t, int64(9), event.NotifyRules[0].Id)
	require.Equal(t, "oncall", event.NotifyRules[0].Name)
}

func TestGetEventNotifyVersion_UsesRuleAndHandlesDeletedRule(t *testing.T) {
	cases := []struct {
		name       string
		ruleExists bool
		ids        []int64
		expected   int
	}{
		{name: "event has notify rules", ids: []int64{9}, expected: 1},
		{name: "rule version", ruleExists: true, expected: 1},
		{name: "deleted rule", expected: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := eventListingRouter(t,
				&models.AlertRule{},
			)
			if tc.ruleExists {
				require.NoError(t, rt.Ctx.DB.Create(&models.AlertRule{Id: 5, NotifyVersion: 1}).Error)
			}

			version, err := GetEventNotifyVersion(rt.Ctx, 5, tc.ids)

			require.NoError(t, err)
			require.Equal(t, tc.expected, version)
		})
	}
}

func TestAlertHisEventsList_FiltersHashAndPaginates(t *testing.T) {
	rt := eventListingRouter(t,
		&models.AlertHisEvent{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedEventListingMembership(t, rt)
	hisTotalCacheMu.Lock()
	originalCache := hisTotalCache
	hisTotalCache = make(map[string]hisTotalCacheEntry)
	hisTotalCacheMu.Unlock()
	t.Cleanup(func() { hisTotalCacheMu.Lock(); hisTotalCache = originalCache; hisTotalCacheMu.Unlock() })
	events := []models.AlertHisEvent{
		{Id: 1, GroupId: 1, Hash: "matching", Severity: 2, RuleProd: "metric", Cate: "prometheus", LastEvalTime: 100},
		{Id: 2, GroupId: 1, Hash: "matching", Severity: 2, RuleProd: "metric", Cate: "prometheus", LastEvalTime: 200},
		{Id: 3, GroupId: 2, Hash: "matching", Severity: 2, RuleProd: "metric", Cate: "prometheus", LastEvalTime: 300},
		{Id: 4, GroupId: 1, Hash: "different", Severity: 2, RuleProd: "metric", Cate: "prometheus", LastEvalTime: 400},
	}
	require.NoError(t, rt.Ctx.DB.Create(&events).Error)
	for page, expectedID := range []int64{2, 1} {
		w := eventListingRequest(t, rt.alertHisEventsList, nil,
			fmt.Sprintf("?hash=matching%%20&severity=2&prods=metric&cate=prometheus&stime=50&etime=500&limit=1&p=%d", page+1), &models.User{
				Id: 1,
			})

		var got struct {
			List  []models.AlertHisEvent `json:"list"`
			Total int                    `json:"total"`
		}
		require.Empty(t, eventListingReply(t, w, &got))
		require.Equal(t, 2, got.Total)
		require.Len(t, got.List, 1)
		require.Equal(t, expectedID, got.List[0].Id)
	}
}
