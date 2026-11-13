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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func recordingBehaviorRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func seedRecordingMembership(t *testing.T, rt *Router) {
	t.Helper()
	db := rt.Ctx.DB
	require.NoError(t, db.Create(&[]models.BusiGroup{{Id: 1, Name: "production"}, {Id: 2, Name: "restricted"}}).Error)
	require.NoError(t, db.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: "rw"}).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
}

func recordingBehaviorRequest(t *testing.T, handler gin.HandlerFunc, body interface{}, query string,
	user *models.User) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("username", "editor"); c.Set("user", user) })
	engine.POST("/groups/:id/rules/:rrid", handler)
	req := httptest.NewRequest(http.MethodPost, "/groups/1/rules/1"+query, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func recordingBehaviorReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
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

func TestRecordingRuleAdd_ReportsValidationPerRuleAndSetsDefaults(t *testing.T) {
	rt := recordingBehaviorRouter(t,
		&models.RecordingRule{},
	)
	rules := []models.RecordingRule{
		{Name: "cpu:rate", PromQl: "rate(cpu[5m])", AppendTagsJSON: []string{"env=prod"}},
		{Name: "invalid metric", PromQl: "up"},
	}

	w := recordingBehaviorRequest(t, rt.recordingRuleAddByFE, rules, "", nil)

	var result map[string]string
	require.Empty(t, recordingBehaviorReply(t, w, &result))
	require.Empty(t, result["cpu:rate"])
	require.Contains(t, result["invalid metric"], "invalid")
	var saved []models.RecordingRule
	require.NoError(t, rt.Ctx.DB.Find(&saved).Error)
	require.Len(t, saved, 1)
	require.Equal(t, int64(1), saved[0].GroupId)
	require.Equal(t, "editor", saved[0].CreateBy)
	require.Equal(t, "@every 60s", saved[0].CronPattern)
	require.Equal(t, "env=prod", saved[0].AppendTags)
	require.Len(t, saved[0].DatasourceQueries, 1)
	require.Equal(t, models.DataSourceQueryAll.MatchType, saved[0].DatasourceQueries[0].MatchType)
}

func TestRecordingRulePut_EnforcesSourceAndDestinationPermissions(t *testing.T) {
	cases := []struct {
		name        string
		source      int64
		destination int64
		allowed     bool
	}{
		{name: "allowed", source: 1, destination: 1, allowed: true},
		{name: "foreign source", source: 2, destination: 1},
		{name: "foreign destination", source: 1, destination: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := recordingBehaviorRouter(t,
				&models.RecordingRule{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedRecordingMembership(t, rt)
			original := models.RecordingRule{Id: 1, Name: "cpu:rate", PromQl: "up", GroupId: tc.source, CreateBy: "creator"}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			replacement := models.RecordingRule{Name: "cpu:rate", PromQl: "up + 1", GroupId: tc.destination}

			w := recordingBehaviorRequest(t, rt.recordingRulePutByFE, replacement, "", &models.User{Id: 1})

			var saved models.RecordingRule
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, original.CreateBy, saved.CreateBy)
			if tc.allowed {
				require.Empty(t, recordingBehaviorReply(t, w, nil))
				require.Equal(t, replacement.PromQl, saved.PromQl)
				require.Equal(t, "editor", saved.UpdateBy)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Equal(t, original.PromQl, saved.PromQl)
				require.Equal(t, original.GroupId, saved.GroupId)
			}
		})
	}
}

func TestRecordingRuleDelete_RestrictsDeletionToRouteGroup(t *testing.T) {
	rt := recordingBehaviorRouter(t,
		&models.RecordingRule{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.RecordingRule{{Id: 1, GroupId: 1}, {Id: 2, GroupId: 2}}).Error)

	w := recordingBehaviorRequest(t, rt.recordingRuleDel, idsForm{Ids: []int64{1, 2}}, "", nil)

	require.Empty(t, recordingBehaviorReply(t, w, nil))
	var saved []models.RecordingRule
	require.NoError(t, rt.Ctx.DB.Find(&saved).Error)
	require.Len(t, saved, 1)
	require.Equal(t, int64(2), saved[0].Id)
}

func TestRecordingRulePutFields_ValidatesCronBeforeWriting(t *testing.T) {
	for _, pattern := range []interface{}{12, "not cron"} {
		t.Run(fmt.Sprint(pattern), func(t *testing.T) {
			rt := recordingBehaviorRouter(t,
				&models.RecordingRule{},
			)
			require.NoError(t, rt.Ctx.DB.Create(&models.RecordingRule{Id: 1, CronPattern: "@every 60s"}).Error)
			form := recordRuleFieldForm{Ids: []int64{1}, Fields: map[string]interface{}{"cron_pattern": pattern}}

			w := recordingBehaviorRequest(t, rt.recordingRulePutFields, form, "", nil)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "cron")
			var saved models.RecordingRule
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, "@every 60s", saved.CronPattern)
		})
	}
}

func TestRecordingRulePutFields_SerializesDatasourceQueriesAndUpdatesAudit(t *testing.T) {
	rt := recordingBehaviorRouter(t,
		&models.RecordingRule{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.RecordingRule{Id: 1, Name: "keep"}).Error)
	queries := []models.DatasourceQuery{{MatchType: 0, Op: "in", Values: []interface{}{float64(3)}}}
	form := recordRuleFieldForm{
		Ids:    []int64{404, 1},
		Fields: map[string]interface{}{"datasource_queries": queries, "disabled": 1, "cron_pattern": "@every 30s"},
	}

	w := recordingBehaviorRequest(t, rt.recordingRulePutFields, form, "", nil)

	require.Empty(t, recordingBehaviorReply(t, w, nil))
	var saved models.RecordingRule
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, queries, saved.DatasourceQueries)
	require.Equal(t, 1, saved.Disabled)
	require.Equal(t, "@every 30s", saved.CronPattern)
	require.Equal(t, "editor", saved.UpdateBy)
	require.Equal(t, "keep", saved.Name)
}

func TestRecordingRuleGetsByGids_RestrictsImplicitMembership(t *testing.T) {
	rt := recordingBehaviorRouter(t,
		&models.RecordingRule{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.User{},
	)
	seedRecordingMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.RecordingRule{
		{Id: 1, GroupId: 1, Name: "own"}, {Id: 2, GroupId: 2, Name: "foreign"},
	}).Error)

	w := recordingBehaviorRequest(t, rt.recordingRuleGetsByGids, nil, "", &models.User{Id: 1})

	var got []models.RecordingRule
	require.Empty(t, recordingBehaviorReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "own", got[0].Name)
}
