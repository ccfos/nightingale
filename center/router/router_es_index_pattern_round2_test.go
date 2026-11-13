package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestEsIndexPatternAdd_StoresSettingsWithOperatorAudit(t *testing.T) {
	rt := identityRouter(t, &models.EsIndexPattern{})
	form := models.EsIndexPattern{
		Name: "logs-*", DatasourceId: 7, TimeField: "@timestamp", AllowHideSystemIndicesBool: true,
		FieldsFormat: "{}", CrossClusterEnabled: 1, Note: "application logs", CreateBy: "forged",
	}

	w := identityRequest(t, rt, rt.esIndexPatternAdd, form, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var got models.EsIndexPattern
	require.NoError(t, rt.Ctx.DB.First(&got).Error)
	require.Equal(t, form.Name, got.Name)
	require.Equal(t, form.DatasourceId, got.DatasourceId)
	require.Equal(t, form.TimeField, got.TimeField)
	require.Equal(t, 1, got.AllowHideSystemIndices)
	require.Equal(t, form.CrossClusterEnabled, got.CrossClusterEnabled)
	require.Equal(t, "operator", got.CreateBy)
	require.Equal(t, "operator", got.UpdateBy)
}

func TestEsIndexPatternAdd_DuplicateLeavesExistingSettingsUnchanged(t *testing.T) {
	rt := identityRouter(t, &models.EsIndexPattern{})
	original := models.EsIndexPattern{Id: 1, DatasourceId: 7, Name: "logs-*", TimeField: "original"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	w := identityRequest(t, rt, rt.esIndexPatternAdd,
		models.EsIndexPattern{DatasourceId: 7, Name: "logs-*", TimeField: "changed"}, "1", nil)

	require.Contains(t, identityReply(t, w, nil), "already exists")
	var got []models.EsIndexPattern
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Equal(t, []models.EsIndexPattern{original}, got)
}

func TestEsIndexPatternPut_UsesQueryIDAndPreservesCreationAndWeight(t *testing.T) {
	rt := identityRouter(t, &models.EsIndexPattern{})
	require.NoError(t, rt.Ctx.DB.Create(&models.EsIndexPattern{
		Id: 1, DatasourceId: 7, Name: "old-*", Weight: 9, CreateBy: "creator", CreateAt: 100,
	}).Error)
	form := models.EsIndexPattern{Id: 999, DatasourceId: 8, Name: "new-*", TimeField: "time", Weight: 0, Note: "updated"}

	w := identityRequest(t, rt, rt.esIndexPatternPut, form, "1?id=1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var got models.EsIndexPattern
	require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
	require.Equal(t, form.Name, got.Name)
	require.Equal(t, form.DatasourceId, got.DatasourceId)
	require.Equal(t, form.TimeField, got.TimeField)
	require.Equal(t, 9, got.Weight)
	require.Equal(t, "creator", got.CreateBy)
	require.Equal(t, int64(100), got.CreateAt)
	require.Equal(t, "operator", got.UpdateBy)
}

func TestEsIndexPatternPut_MissingPatternReturnsNotFound(t *testing.T) {
	rt := identityRouter(t, &models.EsIndexPattern{})

	w := identityRequest(t, rt, rt.esIndexPatternPut, models.EsIndexPattern{Name: "new"}, "1?id=404", nil)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "No such EsIndexPattern")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.EsIndexPattern{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestEsIndexPatternUpdateWeights_ChangesOnlySelectedWeights(t *testing.T) {
	rt := identityRouter(t, &models.EsIndexPattern{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.EsIndexPattern{
		{Id: 1, Name: "one", Weight: 5}, {Id: 2, Name: "two", Weight: 9}, {Id: 3, Name: "three", Weight: 12},
	}).Error)

	w := identityRequest(t, rt, rt.esIndexPatternUpdateWeights,
		[]models.EsIndexPatternWeight{{Id: 1, Weight: 0}, {Id: 2, Weight: 1}}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var got []models.EsIndexPattern
	require.NoError(t, rt.Ctx.DB.Order("id").Find(&got).Error)
	require.Equal(t, 0, got[0].Weight)
	require.Equal(t, 1, got[1].Weight)
	require.Equal(t, 12, got[2].Weight)
	require.Equal(t, "one", got[0].Name)
	require.Equal(t, "two", got[1].Name)
}

func TestEsIndexPatternGetList_FiltersDatasourceAndSortsWeights(t *testing.T) {
	rt := identityRouter(t, &models.EsIndexPattern{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.EsIndexPattern{
		{Id: 1, DatasourceId: 7, Name: "late", Weight: 10},
		{Id: 2, DatasourceId: 7, Name: "first", Weight: 0},
		{Id: 3, DatasourceId: 8, Name: "excluded"},
	}).Error)

	w := identityRequest(t, rt, rt.esIndexPatternGetList, nil, "1?datasource_id=7", nil)

	var got []models.EsIndexPattern
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.Equal(t, "first", got[0].Name)
	require.Equal(t, "late", got[1].Name)
}

func TestEsIndexPatternDel_ProtectsReferencedPatterns(t *testing.T) {
	rt := identityRouter(t, &models.EsIndexPattern{}, &models.AlertRule{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.EsIndexPattern{{Id: 1, Name: "unused"}, {Id: 2, Name: "used"}}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertRule{
		Id: 7, Name: "logs alert", RuleConfig: `{"queries":[{"index_type":"index_pattern","index_pattern":2}]}`,
	}).Error)

	w := identityRequest(t, rt, rt.esIndexPatternDel, idsForm{Ids: []int64{1, 2}}, "1", nil)

	require.Contains(t, identityReply(t, w, nil), "used by alert rules: logs alert")
	var got []models.EsIndexPattern
	require.NoError(t, rt.Ctx.DB.Order("id").Find(&got).Error)
	require.Len(t, got, 2)
	require.Equal(t, "unused", got[0].Name)
	require.Equal(t, "used", got[1].Name)
}

func TestEsIndexPatternDel_DeletesUnusedPatternsAndRejectsEmptyIDs(t *testing.T) {
	rt := identityRouter(t, &models.EsIndexPattern{}, &models.AlertRule{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.EsIndexPattern{{Id: 1}, {Id: 2}}).Error)

	rejected := identityRequest(t, rt, rt.esIndexPatternDel, idsForm{Ids: []int64{}}, "1", nil)

	require.Equal(t, http.StatusBadRequest, rejected.Code)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.EsIndexPattern{}).Count(&count).Error)
	require.Equal(t, int64(2), count)

	w := identityRequest(t, rt, rt.esIndexPatternDel, idsForm{Ids: []int64{1}}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var got []models.EsIndexPattern
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].Id)
}
