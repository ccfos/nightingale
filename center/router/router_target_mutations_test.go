package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func targetMutationRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return &Router{
		Ctx:                   &ctx.Context{DB: db, IsCenter: true},
		TargetDeleteHook:      func(tx *gorm.DB, idents []string, force bool) error { return nil },
		TargetDeleteCheck:     func(idents []string) map[string]string { return nil },
		TargetBgidChangeCheck: func(idents []string, action string, bgids []int64) (map[string]string, error) { return nil, nil },
	}
}

func seedTargetMutationHosts(t *testing.T, rt *Router) {
	t.Helper()
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Target{
		{Ident: "host-a", HostIp: "10.0.0.1", Tags: "env=prod ", Note: "original"},
		{Ident: "host-b", HostIp: "10.0.0.2", Tags: "region=us ", Note: "original"},
	}).Error)
}

func seedTargetMutationBindings(t *testing.T, rt *Router) {
	t.Helper()
	require.NoError(t, rt.Ctx.DB.Create(&[]models.TargetBusiGroup{
		{TargetIdent: "host-a", GroupId: 1}, {TargetIdent: "host-b", GroupId: 2},
	}).Error)
}

func seedTargetMutationMembership(t *testing.T, rt *Router) {
	t.Helper()
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BusiGroup{{Id: 1, Name: "production"}, {Id: 2, Name: "staging"}}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: "rw"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
}

func targetMutationRequest(t *testing.T, handler gin.HandlerFunc, body interface{}, user *models.User) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("user", user) })
	engine.POST("/targets", handler)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)
	return w
}

func targetMutationReply(t *testing.T, w *httptest.ResponseRecorder) (map[string]string, string) {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Dat map[string]string `json:"dat"`
		Err string            `json:"err"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response.Dat, response.Err
}

func TestTargetGets_IgnoresDatasourceIDs(t *testing.T) {
	rt := targetMutationRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	seedTargetMutationHosts(t, rt)
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rt.Redis = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("user", &models.User{RolesLst: []string{models.AdminRole}}) })
	engine.GET("/targets", rt.targetGets)

	// The target table has no datasource_id column since the v6 heartbeat refactor.
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/targets?datasource_ids=3", nil))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var reply struct {
		Err string `json:"err"`
		Dat struct {
			Total int64 `json:"total"`
		} `json:"dat"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reply))
	require.Empty(t, reply.Err)
	require.Equal(t, int64(2), reply.Dat.Total)
}

func TestTargetBindTags_ValidatesTagNamesAndValuesBeforeWriting(t *testing.T) {
	cases := []struct {
		name    string
		tag     string
		message string
	}{
		{name: "missing equals", tag: "env", message: "invalid tag format"},
		{name: "multiple equals", tag: "env=a=b", message: "invalid tag format"},
		{name: "empty key", tag: "=prod", message: "key is empty"},
		{name: "empty value", tag: "env=", message: "value is empty"},
		{name: "dot", tag: "env.name=prod", message: "cannot contain '.'"},
		{name: "hyphen", tag: "env-name=prod", message: "cannot contain '-'"},
		{name: "digit", tag: "1env=prod", message: "must start with a letter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := targetMutationRouter(t,
				&models.Target{},
			)
			seedTargetMutationHosts(t, rt)

			w := targetMutationRequest(t, rt.targetBindTagsByService,
				targetTagsForm{Idents: []string{"host-a"}, Tags: []string{tc.tag}}, nil)

			_, err := targetMutationReply(t, w)
			require.Contains(t, err, tc.message)
			var saved models.Target
			require.NoError(t, rt.Ctx.DB.Where("ident = ?", "host-a").First(&saved).Error)
			require.Equal(t, "env=prod ", saved.Tags)
		})
	}
}

func TestTargetBindTags_AddsTagsAndReportsDuplicatesPerHost(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
	)
	seedTargetMutationHosts(t, rt)
	form := targetTagsForm{Idents: []string{"host-a", "host-b", "missing"}, Tags: []string{"env=stage"}}

	w := targetMutationRequest(t, rt.targetBindTagsByService, form, nil)

	failures, err := targetMutationReply(t, w)
	require.Empty(t, err)
	require.Contains(t, failures["host-a"], "duplicate tagkey(env)")
	require.NotEmpty(t, failures["missing"])
	var a, b models.Target
	require.NoError(t, rt.Ctx.DB.Where("ident = ?", "host-a").First(&a).Error)
	require.NoError(t, rt.Ctx.DB.Where("ident = ?", "host-b").First(&b).Error)
	require.Equal(t, "env=prod ", a.Tags)
	require.ElementsMatch(t, []string{"env=stage", "region=us"}, strings.Fields(b.Tags))
}

func TestTargetBindTags_RejectsMixedUnauthorizedHostsBeforeWriting(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
		&models.TargetBusiGroup{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	seedTargetMutationMembership(t, rt)
	form := targetTagsForm{Idents: []string{"host-a", "host-b"}, Tags: []string{"team=infra"}}

	w := targetMutationRequest(t, rt.targetBindTagsByFE, form, &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	var targets []models.Target
	require.NoError(t, rt.Ctx.DB.Order("ident").Find(&targets).Error)
	require.Equal(t, "env=prod ", targets[0].Tags)
	require.Equal(t, "region=us ", targets[1].Tags)
}

func TestTargetUnbindTags_RemovesOnlyRequestedTags(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
		&models.TargetBusiGroup{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	seedTargetMutationMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Model(&models.Target{}).Where("ident = ?", "host-a").Update("tags", "env=prod region=us ").Error)
	form := targetTagsForm{HostIps: []string{"10.0.0.1"}, Tags: []string{"env=prod"}}

	w := targetMutationRequest(t, rt.targetUnbindTagsByFE, form, &models.User{Id: 1})

	failures, err := targetMutationReply(t, w)
	require.Empty(t, err)
	require.Empty(t, failures)
	var saved models.Target
	require.NoError(t, rt.Ctx.DB.Where("ident = ?", "host-a").First(&saved).Error)
	require.Equal(t, "region=us ", saved.Tags)
}

func TestTargetUpdateNote_ResolvesHostIPsAndEnforcesOwnership(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(fmt.Sprintf("allowed=%t", allowed), func(t *testing.T) {
			rt := targetMutationRouter(t,
				&models.Target{},
				&models.TargetBusiGroup{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedTargetMutationHosts(t, rt)
			seedTargetMutationBindings(t, rt)
			seedTargetMutationMembership(t, rt)
			user := &models.User{Id: 1}
			if !allowed {
				user.Id = 2
			}

			w := targetMutationRequest(t, rt.targetUpdateNote,
				targetNoteForm{HostIps: []string{"10.0.0.1"}, Note: "updated"}, user)

			var saved models.Target
			require.NoError(t, rt.Ctx.DB.Where("ident = ?", "host-a").First(&saved).Error)
			if allowed {
				failures, err := targetMutationReply(t, w)
				require.Empty(t, err)
				require.Empty(t, failures)
				require.Equal(t, "updated", saved.Note)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Equal(t, "original", saved.Note)
			}
		})
	}
}

func TestTargetBindBgids_RejectsUnwritableDestination(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
		&models.TargetBusiGroup{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	seedTargetMutationMembership(t, rt)
	form := targetBgidsForm{Idents: []string{"host-a"}, Bgids: []int64{2}, Action: "add"}

	w := targetMutationRequest(t, rt.targetBindBgids, form, &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	ids, err := models.TargetGroupIdsGetByIdent(rt.Ctx, "host-a")
	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)
}

func TestTargetBindBgids_RequiresBindOperationForUngroupedHosts(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
		&models.TargetBusiGroup{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.RoleOperation{},
	)
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	seedTargetMutationMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Where("target_ident = ?", "host-a").Delete(&models.TargetBusiGroup{}).Error)
	form := targetBgidsForm{Idents: []string{"host-a"}, Bgids: []int64{1}, Action: "add"}

	w := targetMutationRequest(t, rt.targetBindBgids, form, &models.User{Id: 1, RolesLst: []string{"Viewer"}})

	require.Equal(t, http.StatusForbidden, w.Code)
	ids, err := models.TargetGroupIdsGetByIdent(rt.Ctx, "host-a")
	require.NoError(t, err)
	require.Empty(t, ids)
}

func TestTargetBindBgids_RemovalPrecheckPreservesBindings(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
		&models.TargetBusiGroup{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	seedTargetMutationMembership(t, rt)
	rt.TargetBgidChangeCheck = func(idents []string, action string, bgids []int64) (map[string]string, error) {
		require.Equal(t, []string{"host-a"}, idents)
		require.Equal(t, "del", action)
		require.Equal(t, []int64{1}, bgids)
		return map[string]string{"host-a": "still used by collection task"}, nil
	}
	form := targetBgidsForm{Idents: []string{"host-a"}, Bgids: []int64{1}, Action: "del"}

	w := targetMutationRequest(t, rt.targetBindBgids, form, &models.User{Id: 1})

	failures, err := targetMutationReply(t, w)
	require.Empty(t, err)
	require.Equal(t, "still used by collection task", failures["host-a"])
	ids, dbErr := models.TargetGroupIdsGetByIdent(rt.Ctx, "host-a")
	require.NoError(t, dbErr)
	require.Equal(t, []int64{1}, ids)
}

func TestTargetBindBgids_ForcedRemovalSkipsPrecheck(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
		&models.TargetBusiGroup{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	seedTargetMutationMembership(t, rt)
	rt.TargetBgidChangeCheck = func(idents []string, action string, bgids []int64) (map[string]string, error) {
		t.Fatal("forced removal called precheck")
		return nil, nil
	}
	form := targetBgidsForm{Idents: []string{"host-a"}, Bgids: []int64{1}, Action: "del", Force: true}

	w := targetMutationRequest(t, rt.targetBindBgids, form, &models.User{Id: 1})

	failures, err := targetMutationReply(t, w)
	require.Empty(t, err)
	require.Empty(t, failures)
	ids, dbErr := models.TargetGroupIdsGetByIdent(rt.Ctx, "host-a")
	require.NoError(t, dbErr)
	require.Empty(t, ids)
}

func TestTargetBindBgids_RejectsInvalidAction(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
		&models.TargetBusiGroup{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	seedTargetMutationMembership(t, rt)
	form := targetBgidsForm{Idents: []string{"host-a"}, Action: "invalid"}

	w := targetMutationRequest(t, rt.targetBindBgids, form, &models.User{RolesLst: []string{models.AdminRole}})

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "invalid action")
	ids, err := models.TargetGroupIdsGetByIdent(rt.Ctx, "host-a")
	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)
}

func TestTargetDelete_PrecheckPreventsPartialDeletion(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
		&models.TargetBusiGroup{},
	)
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	rt.TargetDeleteCheck = func(idents []string) map[string]string {
		return map[string]string{"host-b": "in use"}
	}

	w := targetMutationRequest(t, rt.targetDel, identsForm{Idents: []string{"host-a", "host-b"}}, nil)

	failures, err := targetMutationReply(t, w)
	require.Empty(t, err)
	require.Equal(t, map[string]string{"host-b": "in use"}, failures)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Target{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestTargetDelete_ForceDeletesHostAndBindings(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
		&models.TargetBusiGroup{},
	)
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	rt.TargetDeleteHook = func(tx *gorm.DB, idents []string, force bool) error {
		require.Equal(t, []string{"host-a"}, idents)
		require.True(t, force)
		return nil
	}

	w := targetMutationRequest(t, rt.targetDel, identsForm{Idents: []string{"host-a"}, Force: true}, nil)

	failures, err := targetMutationReply(t, w)
	require.Empty(t, err)
	require.Empty(t, failures)
	var remaining []models.Target
	require.NoError(t, rt.Ctx.DB.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, "host-b", remaining[0].Ident)
	ids, dbErr := models.TargetGroupIdsGetByIdent(rt.Ctx, "host-a")
	require.NoError(t, dbErr)
	require.Empty(t, ids)
}

func TestTargetDelete_HookFailureRollsBackDeletion(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.Target{},
		&models.TargetBusiGroup{},
	)
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	rt.TargetDeleteHook = func(tx *gorm.DB, idents []string, force bool) error {
		return errors.New("collection task still references host")
	}

	w := targetMutationRequest(t, rt.targetDelByService, identsForm{Idents: []string{"host-a"}}, nil)

	_, err := targetMutationReply(t, w)
	require.Contains(t, err, "collection task still references host")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Target{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
	ids, dbErr := models.TargetGroupIdsGetByIdent(rt.Ctx, "host-a")
	require.NoError(t, dbErr)
	require.Equal(t, []int64{1}, ids)
}

func TestTargetStats_FiltersGroupsAndBucketsUtilization(t *testing.T) {
	rt := targetMutationRouter(t,
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedTargetMutationMembership(t, rt)
	cache := &memsto.TargetCacheType{}
	cache.Set(map[string]*models.Target{
		"host-a": {
			Ident: "host-a",
			GroupIds: []int64{
				1,
			},
			BeatTime:     1,
			CpuNum:       2,
			CpuUtil:      35,
			MemUtil:      75,
			AgentVersion: "v1",
		},
		"host-b":    {Ident: "host-b", GroupIds: []int64{2}, BeatTime: 1, CpuNum: 4, CpuUtil: 95, MemUtil: 10},
		"ungrouped": {Ident: "ungrouped", BeatTime: 1},
	}, 3, 1)
	rt.TargetCache = cache

	w := targetMutationRequest(t, rt.targetStats, nil, &models.User{Id: 1})

	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Dat struct {
			Count    int            `json:"count"`
			Dead     int            `json:"dead_count"`
			CPU      map[string]int `json:"cpu_usage"`
			Mem      map[string]int `json:"mem_usage"`
			Versions map[string]int `json:"versions"`
		} `json:"dat"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	// A member sees their group and unassigned hosts, excluding the foreign group.
	require.Equal(t, 2, response.Dat.Count)
	require.Equal(t, 2, response.Dat.Dead)
	require.Equal(t, 1, response.Dat.CPU["40"])
	require.Equal(t, 1, response.Dat.CPU["-1"])
	require.Equal(t, 1, response.Dat.Mem["80"])
	require.Equal(t, map[string]int{"v1": 1, "unknown": 1}, response.Dat.Versions)
}
