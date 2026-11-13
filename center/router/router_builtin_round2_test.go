package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/center/integration"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func builtinRound2Cache(t *testing.T) *integration.BuiltinPayloadInFileType {
	t.Helper()
	old := integration.BuiltinPayloadInFile
	cache := integration.NewBuiltinPayloadInFileType()
	integration.BuiltinPayloadInFile = cache
	t.Cleanup(func() { integration.BuiltinPayloadInFile = old })
	return cache
}

func TestBuiltinComponentsAdd_ReportsIndividualFailures(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinComponent{})
	require.NoError(t, rt.Ctx.DB.Create(&models.BuiltinComponent{Ident: "existing"}).Error)
	body := []models.BuiltinComponent{{Ident: "custom", Readme: "guide"}, {Ident: "existing"}, {Ident: " "}}

	w := identityRequest(t, rt, rt.builtinComponentsAdd, body, "1", &models.User{Username: "operator"})

	var failures map[string]string
	require.Empty(t, identityReply(t, w, &failures))
	require.Equal(t, map[string]string{"existing": "builtin component already exists", "": "ident is blank"}, failures)
	var saved models.BuiltinComponent
	require.NoError(t, rt.Ctx.DB.Where("ident = ?", "custom").First(&saved).Error)
	require.Equal(t, "guide", saved.Readme)
	require.Equal(t, "operator", saved.CreatedBy)
	require.Equal(t, "operator", saved.UpdatedBy)
}

func TestBuiltinComponentsAdd_RejectsEmptyBatch(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinComponent{})

	w := identityRequest(t, rt, rt.builtinComponentsAdd, []models.BuiltinComponent{},
		"1", &models.User{Username: "operator"})

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "input json is empty")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.BuiltinComponent{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestBuiltinComponentsGets_LocalizesOnlySystemReadmes(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinComponent{})
	cache := builtinRound2Cache(t)
	cache.Readmes["en_US"] = map[string]string{"linux": "English guide", "custom": "must not replace"}
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BuiltinComponent{
		{Ident: "linux", Readme: "source", UpdatedBy: SYSTEM},
		{Ident: "custom", Readme: "user guide", UpdatedBy: "alice"},
		{Ident: "disabled", Disabled: 1, UpdatedBy: SYSTEM},
	}).Error)
	handler := func(c *gin.Context) { c.Request.Header.Set("X-Language", "en_US"); rt.builtinComponentsGets(c) }

	w := identityRequest(t, rt, handler, nil, "1?disabled=0", &models.User{Username: "operator"})

	var got []models.BuiltinComponent
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.Equal(t, "custom", got[0].Ident)
	require.Equal(t, "user guide", got[0].Readme)
	require.Equal(t, "English guide", got[1].Readme)
	var saved models.BuiltinComponent
	require.NoError(t, rt.Ctx.DB.Where("ident = ?", "linux").First(&saved).Error)
	require.Equal(t, "source", saved.Readme)
}

func TestBuiltinComponentsPut_SystemComponentOnlyChangesDisabled(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinComponent{})
	original := models.BuiltinComponent{
		ID:        1,
		Ident:     "linux",
		Readme:    "source",
		Logo:      "logo",
		CreatedBy: SYSTEM,
		UpdatedBy: SYSTEM,
	}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	form := models.BuiltinComponent{ID: 1, Ident: "changed", Readme: "translated", Disabled: 1}

	w := identityRequest(t, rt, rt.builtinComponentsPut, form, "1", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var got models.BuiltinComponent
	require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
	require.Equal(t, 1, got.Disabled)
	require.Equal(t, original.Ident, got.Ident)
	require.Equal(t, original.Readme, got.Readme)
	require.Equal(t, original.Logo, got.Logo)
	require.Equal(t, SYSTEM, got.UpdatedBy)
}

func TestBuiltinComponentsPut_RenamesRelatedMetricTypes(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinComponent{}, &models.BuiltinMetric{})
	original := models.BuiltinComponent{ID: 1, Ident: "old", CreatedBy: "alice"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BuiltinMetric{{ID: 1, Typ: "old"}, {ID: 2, Typ: "other"}}).Error)

	w := identityRequest(t, rt, rt.builtinComponentsPut,
		models.BuiltinComponent{ID: 1, Ident: "new"}, "1", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var got models.BuiltinComponent
	require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
	require.Equal(t, "new", got.Ident)
	var metrics []models.BuiltinMetric
	require.NoError(t, rt.Ctx.DB.Order("id").Find(&metrics).Error)
	require.Equal(t, "new", metrics[0].Typ)
	require.Equal(t, "operator", metrics[0].UpdatedBy)
	require.Equal(t, "other", metrics[1].Typ)
}

func TestBuiltinComponentsPut_DuplicateNameRollsBackMetricRename(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinComponent{}, &models.BuiltinMetric{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BuiltinComponent{
		{ID: 1, Ident: "old", CreatedBy: "alice"}, {ID: 2, Ident: "taken", CreatedBy: "alice"},
	}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.BuiltinMetric{ID: 1, Typ: "old", UpdatedBy: "alice"}).Error)

	w := identityRequest(t, rt, rt.builtinComponentsPut,
		models.BuiltinComponent{ID: 1, Ident: "taken"}, "1", &models.User{Username: "operator"})

	require.Contains(t, identityReply(t, w, nil), "already exists")
	var got models.BuiltinMetric
	require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
	require.Equal(t, "old", got.Typ)
	require.Equal(t, "alice", got.UpdatedBy)
	var component models.BuiltinComponent
	require.NoError(t, rt.Ctx.DB.First(&component, 1).Error)
	require.Equal(t, "old", component.Ident)
}

func TestBuiltinComponentsPut_MissingComponentReturnsNotFound(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinComponent{})

	w := identityRequest(t, rt, rt.builtinComponentsPut,
		models.BuiltinComponent{ID: 404, Ident: "new"}, "1", &models.User{Username: "operator"})

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "No such builtin component")
}

func TestBuiltinComponentsDel_DeletesOnlySelectedComponents(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinComponent{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BuiltinComponent{{ID: 1, Ident: "one"}, {ID: 2, Ident: "two"}}).Error)

	w := identityRequest(t, rt, rt.builtinComponentsDel, idsForm{Ids: []int64{1, 404}},
		"1", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var got []models.BuiltinComponent
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, uint64(2), got[0].ID)
}

func TestBuiltinMetricsAdd_AssignsLanguageAndReportsInvalidItems(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinMetric{})
	valid := models.BuiltinMetric{
		Name: "cpu", Collector: "categraf", Typ: "Linux", Expression: "cpu_usage", Translation: []models.Translation{
			{Lang: "en_US", Name: "CPU usage", Note: "utilization"},
		},
	}
	handler := func(c *gin.Context) { c.Request.Header.Set("X-Language", "en_US"); rt.builtinMetricsAdd(c) }

	w := identityRequest(t, rt, handler, []models.BuiltinMetric{valid, {Name: "invalid"}},
		"1", &models.User{Username: "operator"})

	var failures map[string]string
	require.Empty(t, identityReply(t, w, &failures))
	require.Contains(t, failures["invalid"], "translation is required")
	var got []models.BuiltinMetric
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, "en_US", got[0].Lang)
	require.Positive(t, got[0].UUID)
	require.Equal(t, "operator", got[0].CreatedBy)
	require.Equal(t, valid.Expression, got[0].Expression)
}

func TestBuiltinMetricsPut_PreservesIdentityAndCreationAudit(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinMetric{})
	original := models.BuiltinMetric{ID: 1, UUID: 99, CreatedAt: 123, CreatedBy: "alice"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	form := models.BuiltinMetric{
		ID: 1, Collector: "categraf", Typ: "Linux", Expression: "new_metric", UUID: 1000, CreatedBy: "forged",
		Translation: []models.Translation{{Lang: "en_US", Name: "New metric"}},
	}

	w := identityRequest(t, rt, rt.builtinMetricsPut, form, "1", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var got models.BuiltinMetric
	require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
	require.Equal(t, original.UUID, got.UUID)
	require.Equal(t, original.CreatedAt, got.CreatedAt)
	require.Equal(t, original.CreatedBy, got.CreatedBy)
	require.Equal(t, "operator", got.UpdatedBy)
	require.Equal(t, form.Expression, got.Expression)
}

func TestBuiltinMetricsPut_InvalidMetricLeavesOriginalUnchanged(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinMetric{})
	original := models.BuiltinMetric{ID: 1, Name: "original", UpdatedBy: "alice"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	w := identityRequest(t, rt, rt.builtinMetricsPut,
		models.BuiltinMetric{ID: 1, Name: "bad"}, "1", &models.User{Username: "operator"})

	require.Contains(t, identityReply(t, w, nil), "translation is required")
	var got models.BuiltinMetric
	require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
	require.Equal(t, original.Name, got.Name)
	require.Equal(t, original.UpdatedBy, got.UpdatedBy)
}

func TestBuiltinMetricsDel_DeletesOnlySelectedMetrics(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinMetric{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BuiltinMetric{{ID: 1}, {ID: 2}}).Error)

	w := identityRequest(t, rt, rt.builtinMetricsDel, idsForm{Ids: []int64{1}}, "1", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var got []models.BuiltinMetric
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].ID)
}

func TestBuiltinMetricsGets_FiltersAndPaginatesTranslatedMetrics(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinMetric{})
	builtinRound2Cache(t)
	for i, expression := range []string{"cpu_a", "cpu_b"} {
		metric := models.BuiltinMetric{
			ID: int64(i + 1), Collector: "categraf", Typ: "Linux", Unit: "percent", Expression: expression, UpdatedBy: "alice",
			Translation: []models.Translation{{Lang: "zh_CN", Name: expression, Note: "CPU"}},
		}
		require.NoError(t, rt.Ctx.DB.Create(&metric).Error)
	}
	require.NoError(t, rt.Ctx.DB.Create(&models.BuiltinMetric{Collector: "other", Typ: "Linux"}).Error)

	w := identityRequest(t, rt, rt.builtinMetricsGets, nil,
		"1?collector=categraf&typ=Linux&unit=percent&query=cpu&limit=1&p=2", &models.User{Username: "operator"})

	var got struct {
		List  []models.BuiltinMetric `json:"list"`
		Total int                    `json:"total"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, 2, got.Total)
	require.Len(t, got.List, 1)
	require.Equal(t, "cpu_b", got.List[0].Name)
	require.Equal(t, "cpu_b", got.List[0].Expression)
}

func TestBuiltinMetricFacets_MergeAndSortDatabaseAndFileValues(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinMetric{})
	cache := builtinRound2Cache(t)
	cache.BuiltinMetrics["file"] = &models.BuiltinMetric{Typ: "Linux", Collector: "zagent"}
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BuiltinMetric{
		{Typ: "Linux", Collector: "zagent", UpdatedBy: "alice"},
		{Typ: "Custom", Collector: "agent", UpdatedBy: "alice"},
		{Typ: "Ignored", Collector: "ignored", UpdatedBy: SYSTEM},
	}).Error)
	cases := []struct {
		name     string
		handler  gin.HandlerFunc
		expected []string
	}{
		{name: "types", handler: rt.builtinMetricsTypes, expected: []string{"Custom", "Linux"}},
		{name: "collectors", handler: rt.builtinMetricsCollectors, expected: []string{"agent", "zagent"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, tc.handler, nil, "1", &models.User{Username: "operator"})

			var got []string
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestBuiltinMetricsWrite_RejectsEmptyBatchAndMissingMetric(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinMetric{})
	cases := []struct {
		name    string
		handler gin.HandlerFunc
		body    interface{}
		status  int
		message string
	}{
		{
			name:    "empty import",
			handler: rt.builtinMetricsAdd,
			body:    []models.BuiltinMetric{},
			status:  400,
			message: "input json is empty",
		},
		{
			name:    "missing update",
			handler: rt.builtinMetricsPut,
			body:    models.BuiltinMetric{ID: 404},
			status:  404,
			message: "No such builtin metric",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, tc.handler, tc.body, "1", &models.User{Username: "operator"})

			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.BuiltinMetric{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}
