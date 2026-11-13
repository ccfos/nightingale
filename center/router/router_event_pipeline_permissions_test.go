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

func pipelinePermissionRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func seedPipelinePermissionMembership(t *testing.T, rt *Router) {
	t.Helper()
	db := rt.Ctx.DB
	require.NoError(t, db.Create(&models.UserGroup{Id: 10, Name: "oncall"}).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "production"}).Error)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 2, Name: "restricted"}).Error)
	require.NoError(t, db.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: "rw"}).Error)
}

func pipelinePermissionRequest(t *testing.T, handler gin.HandlerFunc, body interface{}, query string,
	user *models.User) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("user", user) })
	engine.POST("/pipelines/:id", handler)
	req := httptest.NewRequest(http.MethodPost, "/pipelines/1"+query, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func pipelinePermissionReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
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

func TestEventPipelineAdd_EnforcesTeamOrBusinessGroupOwnership(t *testing.T) {
	cases := []struct {
		name     string
		pipeline models.EventPipeline
		allowed  bool
	}{
		{name: "own team", pipeline: models.EventPipeline{Name: "workflow", TeamIds: []int64{10}}, allowed: true},
		{name: "foreign team", pipeline: models.EventPipeline{Name: "workflow", TeamIds: []int64{20}}},
		{name: "own group", pipeline: models.EventPipeline{Name: "workflow", GroupId: 1}, allowed: true},
		{name: "foreign group", pipeline: models.EventPipeline{Name: "workflow", GroupId: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := pipelinePermissionRouter(t,
				&models.EventPipeline{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
				&models.UserGroup{},
			)
			seedPipelinePermissionMembership(t, rt)

			w := pipelinePermissionRequest(t, rt.addEventPipeline, tc.pipeline, "", &models.User{Id: 1, Username: "editor"})

			var saved []models.EventPipeline
			require.NoError(t, rt.Ctx.DB.Find(&saved).Error)
			if tc.allowed {
				var id int64
				require.Empty(t, pipelinePermissionReply(t, w, &id))
				require.Len(t, saved, 1)
				require.Equal(t, saved[0].ID, id)
				require.Equal(t, "editor", saved[0].CreateBy)
				require.Equal(t, "editor", saved[0].UpdateBy)
				require.Equal(t, tc.pipeline.GroupId, saved[0].GroupId)
			} else {
				require.Contains(t, w.Body.String(), "forbidden")
				require.Empty(t, saved)
			}
		})
	}
}

func TestEventPipelineAdd_ValidatesBeforeInsert(t *testing.T) {
	rt := pipelinePermissionRouter(t,
		&models.EventPipeline{},
	)

	w := pipelinePermissionRequest(t, rt.addEventPipeline, models.EventPipeline{
		TeamIds: []int64{
			10,
		},
	}, "", &models.User{
		Id: 1,
	})

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "name cannot be empty")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.EventPipeline{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestEventPipelineUpdate_ChecksBothSourceAndDestinationGroups(t *testing.T) {
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
			rt := pipelinePermissionRouter(t,
				&models.EventPipeline{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
				&models.UserGroup{},
			)
			seedPipelinePermissionMembership(t, rt)
			original := models.EventPipeline{ID: 1, Name: "original", GroupId: tc.source, CreateBy: "creator", CreateAt: 123}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			replacement := models.EventPipeline{ID: 1, Name: "updated", GroupId: tc.destination, CreateBy: "spoofed"}

			w := pipelinePermissionRequest(t, rt.updateEventPipeline, replacement, "", &models.User{Id: 1, Username: "editor"})

			var saved models.EventPipeline
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, original.CreateBy, saved.CreateBy)
			require.Equal(t, original.CreateAt, saved.CreateAt)
			if tc.allowed {
				require.Empty(t, pipelinePermissionReply(t, w, nil))
				require.Equal(t, "updated", saved.Name)
				require.Equal(t, "editor", saved.UpdateBy)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Equal(t, original.Name, saved.Name)
				require.Equal(t, original.GroupId, saved.GroupId)
			}
		})
	}
}

func TestEventPipelineList_FiltersTeamsAndFillsDefaults(t *testing.T) {
	rt := pipelinePermissionRouter(t,
		&models.EventPipeline{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.UserGroup{},
		&models.User{},
	)
	seedPipelinePermissionMembership(t, rt)
	pipelines := []models.EventPipeline{
		{ID: 1, Name: "own", TeamIds: []int64{10}},
		{ID: 2, Name: "other", TeamIds: []int64{20}},
		{ID: 3, Name: "business", GroupId: 1},
	}
	require.NoError(t, rt.Ctx.DB.Create(&pipelines).Error)

	w := pipelinePermissionRequest(t, rt.eventPipelinesList, nil, "", &models.User{Id: 1})

	var got []models.EventPipeline
	require.Empty(t, pipelinePermissionReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].ID)
	require.Equal(t, []string{"oncall"}, got[0].TeamNames)
	require.Equal(t, models.TriggerModeEvent, got[0].TriggerMode)
	require.Equal(t, models.UseCaseEventPipeline, got[0].UseCase)
}

func TestEventPipelineList_RestrictsBusinessGroupsAndUseCase(t *testing.T) {
	for _, member := range []bool{true, false} {
		t.Run(fmt.Sprintf("member=%t", member), func(t *testing.T) {
			rt := pipelinePermissionRouter(t,
				&models.EventPipeline{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
				&models.UserGroup{},
				&models.User{},
			)
			seedPipelinePermissionMembership(t, rt)
			pipelines := []models.EventPipeline{
				{ID: 1, Name: "wanted", GroupId: 1, UseCase: "event_summary"},
				{ID: 2, Name: "different use", GroupId: 1, UseCase: "metric_explorer"},
			}
			require.NoError(t, rt.Ctx.DB.Create(&pipelines).Error)
			user := &models.User{Id: 1}
			if !member {
				user.Id = 2
			}

			w := pipelinePermissionRequest(t, rt.eventPipelinesList, nil, "?group_id=1&use_case=event_summary", user)

			var got []models.EventPipeline
			require.Empty(t, pipelinePermissionReply(t, w, &got))
			if member {
				require.Len(t, got, 1)
				require.Equal(t, "wanted", got[0].Name)
			} else {
				require.Empty(t, got)
			}
		})
	}
}

func TestEventPipelineGet_FillsTeamNamesAndDefaults(t *testing.T) {
	rt := pipelinePermissionRouter(t,
		&models.EventPipeline{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.UserGroup{},
	)
	seedPipelinePermissionMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.EventPipeline{ID: 1, Name: "workflow", TeamIds: []int64{10}}).Error)

	w := pipelinePermissionRequest(t, rt.getEventPipeline, nil, "", &models.User{Id: 1})

	var got models.EventPipeline
	require.Empty(t, pipelinePermissionReply(t, w, &got))
	require.Equal(t, []string{"oncall"}, got.TeamNames)
	require.Equal(t, models.TriggerModeEvent, got.TriggerMode)
	require.Equal(t, models.UseCaseEventPipeline, got.UseCase)
}

func TestEventPipelinesDelete_ChecksEntireBatchBeforeDeleting(t *testing.T) {
	rt := pipelinePermissionRouter(t,
		&models.EventPipeline{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.UserGroup{},
	)
	seedPipelinePermissionMembership(t, rt)
	pipelines := []models.EventPipeline{{ID: 1, Name: "own", GroupId: 1}, {ID: 2, Name: "other", GroupId: 2}}
	require.NoError(t, rt.Ctx.DB.Create(&pipelines).Error)

	w := pipelinePermissionRequest(t, rt.deleteEventPipelines, idsForm{Ids: []int64{1, 2}}, "", &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.EventPipeline{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestEventPipelinesDelete_AllowsWritableGroup(t *testing.T) {
	rt := pipelinePermissionRouter(t,
		&models.EventPipeline{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.UserGroup{},
	)
	seedPipelinePermissionMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.EventPipeline{ID: 1, Name: "own", GroupId: 1}).Error)

	w := pipelinePermissionRequest(t, rt.deleteEventPipelines, idsForm{Ids: []int64{1}}, "", &models.User{Id: 1})

	require.Empty(t, pipelinePermissionReply(t, w, nil))
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.EventPipeline{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestEventPipelinesDisabled_ChangesOnlyStateAndAuditFields(t *testing.T) {
	rt := pipelinePermissionRouter(t,
		&models.EventPipeline{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.UserGroup{},
	)
	seedPipelinePermissionMembership(t, rt)
	original := models.EventPipeline{
		ID: 1, Name: "workflow", GroupId: 1, Description: "preserved", CreateBy: "creator",
		ProcessorConfigs: []models.ProcessorConfig{{Typ: "event_drop", Config: map[string]interface{}{"expression": "true"}}},
	}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	w := pipelinePermissionRequest(t, rt.updateEventPipelinesDisabled,
		json.RawMessage(`{"ids":[1],"disabled":true}`), "", &models.User{Id: 1, Username: "editor"})

	require.Empty(t, pipelinePermissionReply(t, w, nil))
	var saved models.EventPipeline
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.True(t, saved.Disabled)
	require.Equal(t, original.Description, saved.Description)
	require.Equal(t, original.ProcessorConfigs, saved.ProcessorConfigs)
	require.Equal(t, original.CreateBy, saved.CreateBy)
	require.Equal(t, "editor", saved.UpdateBy)
}

func TestEventPipelinesDisabled_RejectsUnauthorizedBatchWithoutWriting(t *testing.T) {
	rt := pipelinePermissionRouter(t,
		&models.EventPipeline{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.UserGroup{},
	)
	seedPipelinePermissionMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.EventPipeline{
		{ID: 1, Name: "own", GroupId: 1}, {ID: 2, Name: "other", GroupId: 2},
	}).Error)

	w := pipelinePermissionRequest(t, rt.updateEventPipelinesDisabled,
		json.RawMessage(`{"ids":[1,2],"disabled":true}`), "", &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.EventPipeline{}).Where("disabled = ?", true).Count(&count).Error)
	require.Zero(t, count)
}

func TestEventPipelineExecutionsList_FiltersAndPaginates(t *testing.T) {
	rt := pipelinePermissionRouter(t,
		&models.EventPipelineExecution{},
	)
	executions := []models.EventPipelineExecution{
		{ID: "first", PipelineID: 1, PipelineName: "workflow", Mode: "api", Status: "success", CreatedAt: 100},
		{ID: "second", PipelineID: 1, PipelineName: "workflow", Mode: "api", Status: "success", CreatedAt: 200},
		{ID: "third", PipelineID: 1, PipelineName: "workflow", Mode: "api", Status: "success", CreatedAt: 300},
		{ID: "failure", PipelineID: 1, PipelineName: "workflow", Mode: "api", Status: "failed", CreatedAt: 250},
	}
	require.NoError(t, rt.Ctx.DB.Create(&executions).Error)

	w := pipelinePermissionRequest(t, rt.listAllEventPipelineExecutions, nil,
		"?pipeline_id=1&pipeline_name=work&mode=api&status=success&stime=100&etime=300&limit=1&p=2", nil)

	var got struct {
		List  []models.EventPipelineExecution `json:"list"`
		Total int                             `json:"total"`
	}
	require.Empty(t, pipelinePermissionReply(t, w, &got))
	require.Equal(t, 3, got.Total)
	require.Len(t, got.List, 1)
	require.Equal(t, "second", got.List[0].ID)
}

func TestEventPipelineExecutionsList_NormalizesInvalidPagination(t *testing.T) {
	rt := pipelinePermissionRouter(t,
		&models.EventPipelineExecution{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.EventPipelineExecution{
		ID:         "execution",
		PipelineID: 1,
		Status:     "success",
	}).Error)

	w := pipelinePermissionRequest(t, rt.listEventPipelineExecutions, nil, "?limit=1001&p=0&status=success", nil)

	var got struct {
		List  []models.EventPipelineExecution `json:"list"`
		Total int                             `json:"total"`
	}
	require.Empty(t, pipelinePermissionReply(t, w, &got))
	require.Equal(t, 1, got.Total)
	require.Len(t, got.List, 1)
	require.Equal(t, "execution", got.List[0].ID)
}
