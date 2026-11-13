package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ruleBehaviorRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func seedRuleBehaviorMembership(t *testing.T, rt *Router) {
	t.Helper()
	db := rt.Ctx.DB
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "production"}).Error)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 2, Name: "staging"}).Error)
	require.NoError(t, db.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: "rw"}).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 7}).Error)
}

func validBehaviorRule(name string) models.AlertRule {
	return models.AlertRule{
		Name:           name,
		Cate:           "prometheus",
		RuleConfigJson: models.PromRuleConfig{Queries: []models.PromQuery{{PromQl: "up == 0", Severity: 2}}},
	}
}

func seedBehaviorRule(t *testing.T, rt *Router, name string, gid int64) models.AlertRule {
	t.Helper()
	rule := validBehaviorRule(name)
	rule.GroupId = gid
	rule.CreateBy = "creator"
	rule.UpdateBy = "creator"
	require.NoError(t, rule.FE2DB())
	require.NoError(t, rule.Add(rt.Ctx))
	return rule
}

func requestBehaviorRule(t *testing.T, rt *Router, handler gin.HandlerFunc, body interface{}, path string,
	user *models.User) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) {
		c.Set("username", "editor")
		c.Set("user", user)
	})
	engine.Any("/groups/:id/rules/:arid", handler)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Language", "en_US")
	engine.ServeHTTP(w, req)
	return w
}

func behaviorRuleReply(t *testing.T, w *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var reply map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reply))
	return reply
}

func TestAlertRuleAdd_UsesRouteGroupAndAuthenticatedAuthor(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	rule := validBehaviorRule("cpu high")
	rule.Id = 99
	rule.GroupId = 999
	rule.CreateBy = "spoofed"

	w := requestBehaviorRule(t, rt, rt.alertRuleAddByFE, []models.AlertRule{rule}, "/groups/1/rules/0", nil)

	reply := behaviorRuleReply(t, w)
	require.JSONEq(t, `{"cpu high":""}`, string(reply["dat"]))
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.NotEqual(t, int64(99), saved.Id)
	require.Equal(t, int64(1), saved.GroupId)
	require.Equal(t, "editor", saved.CreateBy)
	require.Equal(t, "editor", saved.UpdateBy)
	var config models.PromRuleConfig
	require.NoError(t, json.Unmarshal([]byte(saved.RuleConfig), &config))
	require.Len(t, config.Queries, 1)
	require.Equal(t, "up == 0", config.Queries[0].PromQl)
	require.Equal(t, 2, config.Queries[0].Severity)
}

func TestAlertRuleAdd_ReportsValidationAndDuplicateErrorsPerRule(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	seedBehaviorRule(t, rt, "existing", 1)
	invalid := validBehaviorRule("")

	w := requestBehaviorRule(t, rt, rt.alertRuleAddByFE,
		[]models.AlertRule{validBehaviorRule("existing"), invalid, validBehaviorRule("new")}, "/groups/1/rules/0", nil)

	reply := behaviorRuleReply(t, w)
	var result map[string]string
	require.NoError(t, json.Unmarshal(reply["dat"], &result))
	require.Contains(t, result["existing"], "already exists")
	require.Equal(t, "name is blank", result[""])
	require.Empty(t, result["new"])
	var names []string
	require.NoError(t, rt.Ctx.DB.Model(&models.AlertRule{}).Order("name").Pluck("name", &names).Error)
	require.Equal(t, []string{"existing", "new"}, names)
}

func TestAlertRuleAdd_RejectsAllInvalidEffectiveTimesBeforeWriting(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	invalid := validBehaviorRule("broken schedule")
	invalid.EnableStimesJSON = []string{"Invalid date"}

	w := requestBehaviorRule(t, rt, rt.alertRuleAddByFE,
		[]models.AlertRule{validBehaviorRule("valid"), invalid}, "/groups/1/rules/0", nil)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "broken schedule")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AlertRule{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAlertRuleAdd_RejectsEmptyInput(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)

	w := requestBehaviorRule(t, rt, rt.alertRuleAddByFE, []models.AlertRule{}, "/groups/1/rules/0", nil)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "input json is empty")
}

func TestAlertRuleAdd_HookRejectionPreventsInsert(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	old := RuleChangeHook
	t.Cleanup(func() { RuleChangeHook = old })
	RuleChangeHook = func(c *gin.Context, rule interface{}) error { return errors.New("rule quota exceeded") }

	w := requestBehaviorRule(t, rt, rt.alertRuleAddByFE,
		[]models.AlertRule{validBehaviorRule("denied")}, "/groups/1/rules/0", nil)

	reply := behaviorRuleReply(t, w)
	require.JSONEq(t, `{"denied":"rule quota exceeded"}`, string(reply["dat"]))
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AlertRule{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAlertRuleImport_ConvertsLegacyNotifications(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	rule := validBehaviorRule("imported")
	rule.NotifyChannelsJSON = []string{"email"}
	rule.NotifyGroupsJSON = []string{"10"}
	rule.CallbacksJSON = []string{"https://example.invalid/callback"}

	w := requestBehaviorRule(t, rt, rt.alertRuleAddByImport, []models.AlertRule{rule}, "/groups/1/rules/0", nil)

	require.JSONEq(t, `{"imported":""}`, string(behaviorRuleReply(t, w)["dat"]))
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, 1, saved.NotifyVersion)
	require.Empty(t, saved.NotifyChannels)
	require.Empty(t, saved.NotifyGroups)
	require.Empty(t, saved.Callbacks)
	require.Len(t, saved.DatasourceQueries, 1)
	require.Equal(t, models.DataSourceQueryAll.MatchType, saved.DatasourceQueries[0].MatchType)
	require.Equal(t, models.DataSourceQueryAll.Op, saved.DatasourceQueries[0].Op)
	require.Equal(t, fmt.Sprint(models.DataSourceQueryAll.Values), fmt.Sprint(saved.DatasourceQueries[0].Values))
}

func TestAlertRuleImport_ForcePreservesIdentityAndCreationMetadata(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	original := seedBehaviorRule(t, rt, "existing", 1)
	changed := validBehaviorRule("existing")
	changed.Note = "replaced by import"
	changed.Disabled = 1

	w := requestBehaviorRule(t, rt, rt.alertRuleAddByImport,
		[]models.AlertRule{changed, validBehaviorRule("new")}, "/groups/1/rules/0?force=true", nil)

	require.JSONEq(t, `{"existing":"","new":""}`, string(behaviorRuleReply(t, w)["dat"]))
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved, original.Id).Error)
	require.Equal(t, original.CreateAt, saved.CreateAt)
	require.Equal(t, original.CreateBy, saved.CreateBy)
	require.Equal(t, "editor", saved.UpdateBy)
	require.Equal(t, changed.Note, saved.Note)
	require.Equal(t, 1, saved.Disabled)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AlertRule{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestAlertRulePut_ChecksStoredGroupInsteadOfRouteGroup(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(fmt.Sprintf("allowed=%t", allowed), func(t *testing.T) {
			rt := ruleBehaviorRouter(t,
				&models.AlertRule{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedRuleBehaviorMembership(t, rt)
			original := seedBehaviorRule(t, rt, "original", 1)
			user := &models.User{Id: 7}
			if !allowed {
				user.Id = 8
			}
			replacement := validBehaviorRule("updated")
			replacement.GroupId = 2
			replacement.CreateBy = "spoofed"
			path := fmt.Sprintf("/groups/2/rules/%d", original.Id)

			w := requestBehaviorRule(t, rt, rt.alertRulePutByFE, replacement, path, user)

			var saved models.AlertRule
			require.NoError(t, rt.Ctx.DB.First(&saved, original.Id).Error)
			require.Equal(t, int64(1), saved.GroupId)
			require.Equal(t, original.CreateBy, saved.CreateBy)
			if allowed {
				require.JSONEq(t, `""`, string(behaviorRuleReply(t, w)["err"]))
				require.Equal(t, "updated", saved.Name)
				require.Equal(t, "editor", saved.UpdateBy)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Contains(t, w.Body.String(), "forbidden")
				require.Equal(t, original.Name, saved.Name)
				require.Equal(t, original.UpdateBy, saved.UpdateBy)
			}
		})
	}
}

func TestAlertRulePut_HookRejectionPreservesRule(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedRuleBehaviorMembership(t, rt)
	original := seedBehaviorRule(t, rt, "original", 1)
	old := RuleChangeHook
	t.Cleanup(func() { RuleChangeHook = old })
	RuleChangeHook = func(c *gin.Context, rule interface{}) error { return errors.New("rule denied") }

	w := requestBehaviorRule(t, rt, rt.alertRulePutByFE, validBehaviorRule("changed"),
		fmt.Sprintf("/groups/1/rules/%d", original.Id), &models.User{Id: 7})

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "rule denied")
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved, original.Id).Error)
	require.Equal(t, original.Name, saved.Name)
}

func TestAlertRulePut_ValidationErrorPreservesRule(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedRuleBehaviorMembership(t, rt)
	original := seedBehaviorRule(t, rt, "original", 1)
	invalid := validBehaviorRule("changed")
	invalid.AppendTagsJSON = []string{"env=prod", "env=stage"}

	w := requestBehaviorRule(t, rt, rt.alertRulePutByFE, invalid,
		fmt.Sprintf("/groups/1/rules/%d", original.Id), &models.User{Id: 7})

	reply := behaviorRuleReply(t, w)
	require.Contains(t, string(reply["err"]), "duplicate key")
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved, original.Id).Error)
	require.Equal(t, original.Name, saved.Name)
	require.Empty(t, saved.AppendTags)
}

func TestAlertRuleGetAndPut_ReturnNotFound(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Router) gin.HandlerFunc
	}{
		{name: "get", handler: func(rt *Router) gin.HandlerFunc { return rt.alertRuleGet }},
		{name: "pure get", handler: func(rt *Router) gin.HandlerFunc { return rt.alertRulePureGet }},
		{name: "update", handler: func(rt *Router) gin.HandlerFunc { return rt.alertRulePutByFE }},
		{name: "service update", handler: func(rt *Router) gin.HandlerFunc { return rt.alertRulePutByService }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := ruleBehaviorRouter(t,
				&models.AlertRule{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedRuleBehaviorMembership(t, rt)

			w := requestBehaviorRule(t, rt, tc.handler(rt), validBehaviorRule("missing"), "/groups/1/rules/404", nil)

			require.Equal(t, http.StatusNotFound, w.Code)
			require.Equal(t, "No such AlertRule", w.Body.String())
		})
	}
}

func TestAlertRuleDelete_RestrictsDeletionToRouteGroup(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	own := seedBehaviorRule(t, rt, "own", 1)
	other := seedBehaviorRule(t, rt, "other", 2)

	w := requestBehaviorRule(t, rt, rt.alertRuleDel, idsForm{Ids: []int64{own.Id, other.Id}}, "/groups/1/rules/0", nil)

	require.JSONEq(t, `""`, string(behaviorRuleReply(t, w)["err"]))
	var remaining []models.AlertRule
	require.NoError(t, rt.Ctx.DB.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, other.Id, remaining[0].Id)
}

func TestAlertRuleDelete_RejectsEmptyIDs(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	original := seedBehaviorRule(t, rt, "keep", 1)

	w := requestBehaviorRule(t, rt, rt.alertRuleDel, idsForm{}, "/groups/1/rules/0", nil)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "ids empty")
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved, original.Id).Error)
}

func TestAlertRulePutFields_ValidatesCronBeforeWriting(t *testing.T) {
	cases := []struct {
		name    string
		pattern interface{}
		status  int
		message string
	}{
		{name: "wrong type", pattern: 12, status: http.StatusBadRequest, message: "cron_pattern must be a string"},
		{name: "invalid schedule", pattern: "not cron", status: http.StatusBadRequest, message: "cron"},
		{name: "valid schedule", pattern: "0 */5 * * * *", status: http.StatusOK},
		{name: "clear schedule", pattern: "", status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := ruleBehaviorRouter(t,
				&models.AlertRule{},
			)
			original := seedBehaviorRule(t, rt, "keep", 1)
			require.NoError(t, rt.Ctx.DB.Model(&original).Update("cron_pattern", "0 0 * * * *").Error)
			form := alertRuleFieldForm{Ids: []int64{original.Id}, Fields: map[string]interface{}{"cron_pattern": tc.pattern}}

			w := requestBehaviorRule(t, rt, rt.alertRulePutFields, form, "/groups/1/rules/0", nil)

			require.Equal(t, tc.status, w.Code, w.Body.String())
			var saved models.AlertRule
			require.NoError(t, rt.Ctx.DB.First(&saved, original.Id).Error)
			if tc.status == http.StatusOK {
				require.Equal(t, tc.pattern, saved.CronPattern)
				require.Equal(t, "editor", saved.UpdateBy)
			} else {
				require.Contains(t, w.Body.String(), tc.message)
				require.Equal(t, "0 0 * * * *", saved.CronPattern)
				require.Equal(t, original.UpdateBy, saved.UpdateBy)
			}
		})
	}
}

func TestAlertRulePutFields_ChangesDisabledStateAndSkipsMissingRules(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	original := seedBehaviorRule(t, rt, "toggle", 1)
	for _, disabled := range []int{1, 0} {
		form := alertRuleFieldForm{Ids: []int64{404, original.Id}, Fields: map[string]interface{}{"disabled": disabled}}

		w := requestBehaviorRule(t, rt, rt.alertRulePutFields, form, "/groups/1/rules/0", nil)

		require.JSONEq(t, `""`, string(behaviorRuleReply(t, w)["err"]))
		var saved models.AlertRule
		require.NoError(t, rt.Ctx.DB.First(&saved, original.Id).Error)
		require.Equal(t, disabled, saved.Disabled)
		require.Equal(t, "editor", saved.UpdateBy)
	}
}

func TestAlertRulePutFields_ChangesDatasourceQueries(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	original := seedBehaviorRule(t, rt, "datasources", 1)
	queries := []models.DatasourceQuery{{MatchType: 0, Op: "in", Values: []interface{}{float64(3)}}}
	form := alertRuleFieldForm{
		Ids: []int64{original.Id}, Action: "datasource_change",
		Fields: map[string]interface{}{"datasource_queries": queries},
	}

	w := requestBehaviorRule(t, rt, rt.alertRulePutFields, form, "/groups/1/rules/0", nil)

	require.JSONEq(t, `""`, string(behaviorRuleReply(t, w)["err"]))
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved, original.Id).Error)
	require.Equal(t, queries, saved.DatasourceQueries)
}

func TestBatchAlertRuleClone_ClonesIntoWritableGroupsAndReportsMissingRules(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedRuleBehaviorMembership(t, rt)
	original := seedBehaviorRule(t, rt, "source", 2)
	form := alertBatchCloneForm{RuleIds: []int64{original.Id, 404}, Bgids: []int64{1}}

	w := requestBehaviorRule(t, rt, rt.batchAlertRuleClone, form, "/groups/1/rules/0", &models.User{
		Id:       7,
		Username: "cloner",
	})

	reply := behaviorRuleReply(t, w)
	var result map[string]string
	require.NoError(t, json.Unmarshal(reply["dat"], &result))
	require.Contains(t, result["404-1"], "alert rule not found")
	require.Len(t, result, 1)
	var cloned models.AlertRule
	require.NoError(t, rt.Ctx.DB.Where("group_id = ?", 1).First(&cloned).Error)
	require.NotEqual(t, original.Id, cloned.Id)
	require.Equal(t, original.Name, cloned.Name)
	require.Equal(t, "cloner", cloned.CreateBy)
	require.JSONEq(t, original.RuleConfig, cloned.RuleConfig)
}

func TestBatchAlertRuleClone_RejectsUnwritableDestination(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedRuleBehaviorMembership(t, rt)
	original := seedBehaviorRule(t, rt, "source", 1)
	form := alertBatchCloneForm{RuleIds: []int64{original.Id}, Bgids: []int64{2}}

	w := requestBehaviorRule(t, rt, rt.batchAlertRuleClone, form, "/groups/1/rules/0", &models.User{Id: 7})

	require.Equal(t, http.StatusForbidden, w.Code)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AlertRule{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestRelabelTest_AppliesDefaultRegexAndSeparator(t *testing.T) {
	rt := &Router{}
	body := json.RawMessage(`{
  "tags":["env=prod","region=us-east"],
  "configs":[{"action":"replace","source_labels":["env","region"],"target_label":"deployment"}]
 }`)

	w := requestBehaviorRule(t, rt, rt.relabelTest, body, "/groups/1/rules/0", nil)

	var tags []string
	require.NoError(t, json.Unmarshal(behaviorRuleReply(t, w)["dat"], &tags))
	require.ElementsMatch(t, []string{"env=prod", "region=us-east", "deployment=prod;us-east"}, tags)
}

func TestRelabelTest_RejectsMissingConfigurationAndMalformedTags(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{name: "no config", body: `{"tags":["env=prod"]}`, message: "relabel config is empty"},
		{name: "bad tag", body: `{"tags":["env"],"configs":[{"action":"drop"}]}`, message: "tag:env format error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}

			w := requestBehaviorRule(t, rt, rt.relabelTest, json.RawMessage(tc.body), "/groups/1/rules/0", nil)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
		})
	}
}

func TestAlertRuleServiceAdd_ResetsCallerIDsAndKeepsRequestedGroup(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	rule := validBehaviorRule("service rule")
	rule.Id = 99
	rule.GroupId = 2
	rule.CreateBy = "service"

	w := requestBehaviorRule(t, rt, rt.alertRuleAddByService, []models.AlertRule{rule}, "/groups/1/rules/0", nil)

	require.JSONEq(t, `{"service rule":""}`, string(behaviorRuleReply(t, w)["dat"]))
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.NotEqual(t, int64(99), saved.Id)
	require.Equal(t, int64(2), saved.GroupId)
	require.Equal(t, "service", saved.CreateBy)
}

func TestAlertRuleServiceAddOne_ReturnsSavedID(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	rule := validBehaviorRule("single rule")
	rule.GroupId = 1

	w := requestBehaviorRule(t, rt, rt.alertRuleAddOneByService, rule, "/groups/1/rules/0", nil)

	var id int64
	require.NoError(t, json.Unmarshal(behaviorRuleReply(t, w)["dat"], &id))
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved, id).Error)
	require.Equal(t, "single rule", saved.Name)
}

func TestAlertRuleServicePut_PreservesGroupAndCreator(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	original := seedBehaviorRule(t, rt, "original", 1)
	replacement := validBehaviorRule("updated")
	replacement.GroupId = 2
	replacement.CreateBy = "spoofed"

	w := requestBehaviorRule(t, rt, rt.alertRulePutByService, replacement,
		fmt.Sprintf("/groups/2/rules/%d", original.Id), nil)

	require.JSONEq(t, `""`, string(behaviorRuleReply(t, w)["err"]))
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved, original.Id).Error)
	require.Equal(t, "updated", saved.Name)
	require.Equal(t, original.GroupId, saved.GroupId)
	require.Equal(t, original.CreateBy, saved.CreateBy)
}

func TestAlertRuleCloneToMachine_ReplacesIdentAndReportsDuplicates(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	rule := validBehaviorRule("host cpu")
	rule.GroupId = 1
	rule.RuleConfigJson = models.PromRuleConfig{
		Queries: []models.PromQuery{{PromQl: `cpu_usage{ident="old"} > 80`, Severity: 2}},
	}
	require.NoError(t, rule.FE2DB())
	require.NoError(t, rule.Add(rt.Ctx))
	form := identListForm{Ids: []int64{rule.Id}, IdentList: []string{"host-a", "host-b"}}

	w := requestBehaviorRule(t, rt, rt.cloneToMachine, form, "/groups/1/rules/0", nil)

	require.JSONEq(t, `{}`, string(behaviorRuleReply(t, w)["dat"]))
	for _, ident := range form.IdentList {
		var clone models.AlertRule
		require.NoError(t, rt.Ctx.DB.Where("name = ?", "host cpu_"+ident).First(&clone).Error)
		var config models.PromRuleConfig
		require.NoError(t, json.Unmarshal([]byte(clone.RuleConfig), &config))
		require.Equal(t, `cpu_usage{ident="`+ident+`"} > 80`, config.Queries[0].PromQl)
		require.Equal(t, "editor", clone.CreateBy)
	}

	w = requestBehaviorRule(t, rt, rt.cloneToMachine, form, "/groups/1/rules/0", nil)

	var failures map[string]map[string]string
	require.NoError(t, json.Unmarshal(behaviorRuleReply(t, w)["dat"], &failures))
	require.Contains(t, failures["host cpu"]["host-a"], "already exists")
	require.Contains(t, failures["host cpu"]["host-b"], "already exists")
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved, rule.Id).Error)
	require.JSONEq(t, rule.RuleConfig, saved.RuleConfig)
}

func TestAlertRuleCloneToMachine_RejectsUnsupportedRules(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	rules := []models.AlertRule{
		{Id: 1, Name: "database", Cate: "mysql", RuleConfig: `{}`},
		{Id: 2, Name: "regex host", Cate: "prometheus", RuleConfig: `{"queries":[{"prom_ql":"up{ident=~\"host.*\"}"}]}`},
	}
	require.NoError(t, rt.Ctx.DB.Create(&rules).Error)

	w := requestBehaviorRule(t, rt, rt.cloneToMachine,
		identListForm{Ids: []int64{1, 2}, IdentList: []string{"host-a"}}, "/groups/1/rules/0", nil)

	var failures map[string]map[string]string
	require.NoError(t, json.Unmarshal(behaviorRuleReply(t, w)["dat"], &failures))
	require.Contains(t, failures["database"]["all"], "Only Prometheus")
	require.Contains(t, failures["regex host"]["all"], "missing ident")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.AlertRule{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestAlertRulePureGet_ReturnsDecodedConfiguration(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	rule := seedBehaviorRule(t, rt, "read", 1)

	w := requestBehaviorRule(t, rt, rt.alertRulePureGet, nil, fmt.Sprintf("/groups/1/rules/%d", rule.Id), nil)

	var got models.AlertRule
	require.NoError(t, json.Unmarshal(behaviorRuleReply(t, w)["dat"], &got))
	require.Equal(t, rule.Id, got.Id)
	require.Equal(t, rule.Name, got.Name)
	require.NotNil(t, got.RuleConfigJson)
}

func TestAlertRuleGetsByGids_FiltersMembershipAndCountsEventsInRange(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.AlertCurEvent{},
		&models.User{},
	)
	seedRuleBehaviorMembership(t, rt)
	own := seedBehaviorRule(t, rt, "own", 1)
	seedBehaviorRule(t, rt, "foreign", 2)
	events := []models.AlertCurEvent{
		{RuleId: own.Id, TriggerTime: 100}, {RuleId: own.Id, TriggerTime: 200}, {RuleId: own.Id, TriggerTime: 400},
	}
	require.NoError(t, rt.Ctx.DB.Create(&events).Error)

	w := requestBehaviorRule(t, rt, rt.alertRuleGetsByGids, nil, "/groups/1/rules/0?stime=50&etime=300", &models.User{
		Id: 7,
	})

	var got []models.AlertRule
	require.NoError(t, json.Unmarshal(behaviorRuleReply(t, w)["dat"], &got))
	require.Len(t, got, 1)
	require.Equal(t, own.Id, got[0].Id)
	require.EqualValues(t, 2, got[0].CurEventCount)
}

func TestAlertRuleGetsByGids_RejectsForeignExplicitGroup(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedRuleBehaviorMembership(t, rt)

	w := requestBehaviorRule(t, rt, rt.alertRuleGetsByGids, nil, "/groups/1/rules/0?gids=2", &models.User{Id: 7})

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "forbidden")
}

func TestAlertRuleGetsByGids_ReturnsEmptyForUserWithoutGroups(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedRuleBehaviorMembership(t, rt)
	seedBehaviorRule(t, rt, "private", 1)

	w := requestBehaviorRule(t, rt, rt.alertRuleGetsByGids, nil, "/groups/1/rules/0", &models.User{Id: 8})

	require.JSONEq(t, `[]`, string(behaviorRuleReply(t, w)["dat"]))
}

func TestAlertRulesGetByService_AppliesProductCategoryAndDisabledFilters(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
		&models.User{},
	)
	rules := []models.AlertRule{
		{Name: "wanted", Prod: "metric", Cate: "prometheus", AppendTags: "env=prod"},
		{Name: "disabled", Prod: "metric", Cate: "prometheus", AppendTags: "env=prod", Disabled: 1},
		{Name: "other product", Prod: "host", Cate: "prometheus", AppendTags: "env=prod"},
		{Name: "other cate", Prod: "metric", Cate: "mysql", AppendTags: "env=prod"},
	}
	require.NoError(t, rt.Ctx.DB.Create(&rules).Error)

	w := requestBehaviorRule(t, rt, rt.alertRulesGetByService, nil,
		"/groups/1/rules/0?prods=metric&cate=prometheus&disabled=0&query=env%3Dprod", nil)

	var got []models.AlertRule
	require.NoError(t, json.Unmarshal(behaviorRuleReply(t, w)["dat"], &got))
	require.Len(t, got, 1)
	require.Equal(t, "wanted", got[0].Name)
}

func TestAlertRuleValidation_RequiresRecipientTokenForEachBuiltinChannel(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprintf("configured=%t", configured), func(t *testing.T) {
			users := &memsto.UserCacheType{}
			user := &models.User{Id: 1, Username: "recipient"}
			if configured {
				user.Email = "recipient@example.invalid"
			}
			users.Set(map[int64]*models.User{1: user}, 1, 1, 0, 0)
			groups := &memsto.UserGroupCacheType{}
			groups.Set(map[int64]*models.UserGroup{10: {Id: 10, UserIds: []int64{1}}}, 1, 1)
			rt := &Router{UserCache: users, UserGroupCache: groups}
			rule := models.AlertRule{NotifyChannelsJSON: []string{models.Email, "custom"}, NotifyGroupsJSON: []string{"10"}}

			w := requestBehaviorRule(t, rt, rt.alertRuleValidation, rule, "/groups/1/rules/0", nil)

			reply := behaviorRuleReply(t, w)
			if configured {
				require.JSONEq(t, `""`, string(reply["err"]))
			} else {
				require.Contains(t, string(reply["err"]), "missing notify channel configurations")
				require.Contains(t, string(reply["err"]), "email")
			}
		})
	}
}

func TestAlertRuleCallbacks_ReturnsDistinctCallbacksFromOwnGroups(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedRuleBehaviorMembership(t, rt)
	rules := []models.AlertRule{
		{Name: "one", GroupId: 1, Callbacks: "http://one.invalid http://shared.invalid"},
		{Name: "two", GroupId: 1, Callbacks: "http://shared.invalid"},
		{Name: "foreign", GroupId: 2, Callbacks: "http://foreign.invalid"},
	}
	require.NoError(t, rt.Ctx.DB.Create(&rules).Error)

	w := requestBehaviorRule(t, rt, rt.alertRuleCallbacks, nil, "/groups/1/rules/0", &models.User{Id: 7})

	var got []string
	require.NoError(t, json.Unmarshal(behaviorRuleReply(t, w)["dat"], &got))
	require.ElementsMatch(t, []string{"http://one.invalid", "http://shared.invalid"}, got)
}

func TestAlertRuleNotifyTryRun_ValidatesBuiltinRecipientContactsWithoutSending(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprintf("configured=%t", configured), func(t *testing.T) {
			rt := ruleBehaviorRouter(t,
				&models.AlertRule{},
			)
			require.NoError(t, rt.Ctx.DB.AutoMigrate(&models.AlertHisEvent{}))
			require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{
				Id:             1,
				NotifyChannels: models.Email,
				Tags:           "env=prod",
			}).Error)
			users := &memsto.UserCacheType{}
			user := &models.User{Id: 1}
			if configured {
				user.Email = "recipient@example.invalid"
			}
			users.Set(map[int64]*models.User{1: user}, 1, 1, 0, 0)
			groups := &memsto.UserGroupCacheType{}
			groups.Set(map[int64]*models.UserGroup{10: {Id: 10, UserIds: []int64{1}}}, 1, 1)
			rt.UserCache = users
			rt.UserGroupCache = groups
			form := AlertRuleTryRunForm{
				EventId: 1, AlertRuleConfig: models.AlertRule{
					NotifyChannelsJSON: []string{
						models.Email,
					},
					NotifyGroupsJSON: []string{
						"10",
					},
				},
			}

			w := requestBehaviorRule(t, rt, rt.alertRuleNotifyTryRun, form, "/groups/1/rules/0", nil)

			reply := behaviorRuleReply(t, w)
			if configured {
				require.JSONEq(t, `"notification test ok"`, string(reply["dat"]))
			} else {
				require.Contains(t, string(reply["err"]), "missing notify channel configurations")
			}
		})
	}
}

func TestAlertRuleEnableTryRun_ReportsDisabledAndEffectiveRules(t *testing.T) {
	for _, disabled := range []int{1, 0} {
		t.Run(fmt.Sprintf("disabled=%d", disabled), func(t *testing.T) {
			rt := ruleBehaviorRouter(t,
				&models.AlertRule{},
			)
			require.NoError(t, rt.Ctx.DB.AutoMigrate(&models.AlertHisEvent{}))
			require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{Id: 1, TriggerTime: 100, Tags: "env=prod"}).Error)
			form := AlertRuleTryRunForm{EventId: 1, AlertRuleConfig: models.AlertRule{Disabled: disabled}}

			w := requestBehaviorRule(t, rt, rt.alertRuleEnableTryRun, form, "/groups/1/rules/0", nil)

			reply := behaviorRuleReply(t, w)
			if disabled == 1 {
				require.Contains(t, string(reply["err"]), "rule is disabled")
			} else {
				require.JSONEq(t, `"event is effective"`, string(reply["dat"]))
			}
		})
	}
}

func TestAlertRuleImportPromRule_ConvertsYAMLIntoAlertRule(t *testing.T) {
	rt := ruleBehaviorRouter(t,
		&models.AlertRule{},
	)
	form := promRuleForm{
		Payload:           "groups:\n- name: host\n  rules:\n  - alert: HostDown\n    expr: up == 0\n    for: 1m\n",
		DatasourceQueries: []models.DatasourceQuery{models.DataSourceQueryAll}, Disabled: 1,
	}

	w := requestBehaviorRule(t, rt, rt.alertRuleAddByImportPromRule, form, "/groups/1/rules/0", nil)

	require.JSONEq(t, `{"HostDown":""}`, string(behaviorRuleReply(t, w)["dat"]))
	var saved models.AlertRule
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, "HostDown", saved.Name)
	require.Equal(t, 1, saved.Disabled)
	require.Equal(t, "editor", saved.CreateBy)
	var config models.PromRuleConfig
	require.NoError(t, json.Unmarshal([]byte(saved.RuleConfig), &config))
	require.Equal(t, "up == 0", config.Queries[0].PromQl)
	require.Equal(t, 60, saved.PromForDuration)
}
