package chat

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newPreflightTestDeps(t *testing.T, tables ...interface{}) *aiagent.ToolDeps {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(tables...))
	return &aiagent.ToolDeps{DBCtx: ctxpkg.NewContext(context.Background(), db, true)}
}

func TestPreflightCreation_ResolvesNamedVisibleResources(t *testing.T) {
	deps := newPreflightTestDeps(t, &models.BusiGroup{}, &models.Datasource{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 7, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&[]models.Datasource{
		{Id: 11, Name: "metrics"}, {Id: 12, Name: "secret"},
	}).Error)
	deps.FilterDatasources = func(all []*models.Datasource, _ *models.User) []*models.Datasource {
		for _, ds := range all {
			if ds.Id == 11 {
				return []*models.Datasource{ds}
			}
		}
		return nil
	}
	req := &AIChatRequest{UserInput: "create alert rule in Operations using metrics"}
	user := &models.User{RolesLst: []string{models.AdminRole}}

	halt, responses, err := PreflightCreation(context.Background(), deps, req, user)

	require.NoError(t, err)
	require.False(t, halt)
	require.Empty(t, responses)
	require.Equal(t, int64(7), req.Context["busi_group_id"])
	require.Equal(t, int64(11), req.Context["datasource_id"])
}

func TestPreflightCreation_MissingDatasourceOffersAllFieldsWithGroupPreselected(t *testing.T) {
	deps := newPreflightTestDeps(t, &models.BusiGroup{}, &models.Datasource{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 7, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.Datasource{Id: 11, Name: "metrics", PluginType: "prometheus"}).Error)
	deps.FilterDatasources = func(all []*models.Datasource, _ *models.User) []*models.Datasource { return all }
	req := &AIChatRequest{UserInput: "create alert rule", Context: map[string]interface{}{"busi_group_id": int64(7)}}
	user := &models.User{RolesLst: []string{models.AdminRole}}

	halt, responses, err := PreflightCreation(context.Background(), deps, req, user)

	require.NoError(t, err)
	require.True(t, halt)
	require.Len(t, responses, 1)
	require.Equal(t, models.ContentTypeFormSelect, responses[0].ContentType)
	var form aiagent.FormPayload
	require.NoError(t, json.Unmarshal([]byte(responses[0].Content), &form))
	require.Equal(t, aiagent.FormPayload{SkillName: "create-alert-rule", Fields: []aiagent.FormField{
		{
			Key: "busi_group_id", Type: "single",
			Candidates: []aiagent.FormCandidate{{ID: 7, Name: "Operations", IsDefault: true}},
		},
		{
			Key: "datasource_id", Type: "single",
			Candidates: []aiagent.FormCandidate{{ID: 11, Name: "metrics", Extra: "prometheus"}},
		},
	}}, form)
}

func TestPreflightCreation_UnmatchedInputDoesNotNeedDependencies(t *testing.T) {
	req := &AIChatRequest{UserInput: "Explain histograms"}

	halt, responses, err := PreflightCreation(context.Background(), nil, req, nil)

	require.NoError(t, err)
	require.False(t, halt)
	require.Empty(t, responses)
	require.Nil(t, req.Context)
}

func TestPreflightCreation_CompleteContextProceedsWithoutLookup(t *testing.T) {
	req := &AIChatRequest{UserInput: "create alert rule", Context: map[string]interface{}{
		"busi_group_id": int64(7), "datasource_id": int64(11),
	}}

	halt, responses, err := PreflightCreation(context.Background(), nil, req, nil)

	require.NoError(t, err)
	require.False(t, halt)
	require.Empty(t, responses)
}
