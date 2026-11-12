package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPayloadsAdd_ImportsAlertRuleMetadata(t *testing.T) {
	cases := []struct {
		name          string
		content       string
		expectedNames []string
	}{
		{name: "single", content: `{"name":"CPU high","uuid":123,"append_tags":["env=prod","team=ops"]}`, expectedNames: []string{"CPU high"}},
		{
			name:          "array",
			content:       `[{"name":"CPU high","uuid":123,"append_tags":["env=prod","team=ops"]},{"name":"CPU low","uuid":124}]`,
			expectedNames: []string{"CPU high", "CPU low"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.BuiltinPayload{})
			body := []models.BuiltinPayload{{Type: "alert", ComponentID: 7, Cate: "prometheus", Content: tc.content}}

			w := identityRequest(t, rt, rt.builtinPayloadsAdd, body, "1", &models.User{Username: "operator"})

			var failures map[string]string
			require.Empty(t, identityReply(t, w, &failures))
			require.Empty(t, failures)
			var saved []models.BuiltinPayload
			require.NoError(t, rt.Ctx.DB.Order("id").Find(&saved).Error)
			require.Len(t, saved, len(tc.expectedNames))
			for i, payload := range saved {
				require.Equal(t, tc.expectedNames[i], payload.Name)
				require.Equal(t, "alert", payload.Type)
				require.Equal(t, uint64(7), payload.ComponentID)
				require.Equal(t, "prometheus", payload.Cate)
				require.Equal(t, "operator", payload.CreatedBy)
				var rule models.AlertRule
				require.NoError(t, json.Unmarshal([]byte(payload.Content), &rule))
				require.Equal(t, rule.UUID, payload.UUID)
				// AppendTags is json:"-"; the tags arrive as append_tags.
				require.Equal(t, strings.Join(rule.AppendTagsJSON, " "), payload.Tags)
			}
			require.Equal(t, "env=prod team=ops", saved[0].Tags)
		})
	}
}

func TestBuiltinPayloadsAdd_ImportsDashboardMetadata(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "single", content: `{"name":"Overview","tags":"prod","note":"readme","configs":{"panels":[]}}`},
		{name: "array", content: `[{"name":"Overview","tags":"prod","note":"readme","configs":{"panels":[]}}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.BuiltinPayload{})
			body := []models.BuiltinPayload{{Type: "dashboard", ComponentID: 7, Content: tc.content}}

			w := identityRequest(t, rt, rt.builtinPayloadsAdd, body, "1", &models.User{Username: "operator"})

			var failures map[string]string
			require.Empty(t, identityReply(t, w, &failures))
			require.Empty(t, failures)
			var saved models.BuiltinPayload
			require.NoError(t, rt.Ctx.DB.First(&saved).Error)
			require.Equal(t, "Overview", saved.Name)
			require.Equal(t, "prod", saved.Tags)
			require.Equal(t, "readme", saved.Note)
			require.Positive(t, saved.UUID)
			var board Board
			require.NoError(t, json.Unmarshal([]byte(saved.Content), &board))
			require.Equal(t, saved.UUID, board.UUID)
			require.Equal(t, map[string]interface{}{"panels": []interface{}{}}, board.Configs)
		})
	}
}

func TestBuiltinPayloadsAdd_ReportsMalformedContentWithoutSavingIt(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		content string
	}{
		{name: "invalid alert", typ: "alert", content: "{"},
		{name: "invalid alert array", typ: "alert", content: "["},
		{name: "invalid dashboard", typ: "dashboard", content: "{"},
		{name: "invalid dashboard array", typ: "dashboard", content: "["},
		{name: "invalid collect", typ: "collect", content: "hello world"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.BuiltinPayload{})
			body := []models.BuiltinPayload{{Type: tc.typ, Name: "broken", ComponentID: 7, Content: tc.content}}

			w := identityRequest(t, rt, rt.builtinPayloadsAdd, body, "1", &models.User{Username: "operator"})

			var failures map[string]string
			require.Empty(t, identityReply(t, w, &failures))
			require.NotEmpty(t, failures["broken"])
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.BuiltinPayload{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestBuiltinPayloadsAdd_ValidatesAndSavesCollectionTemplates(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinPayload{})
	body := []models.BuiltinPayload{
		{Type: "collect", ComponentID: 7, Name: "agent", Content: "interval = 15", UUID: 123},
		{Type: "collect", Name: "invalid", Content: "interval = 15"},
	}

	w := identityRequest(t, rt, rt.builtinPayloadsAdd, body, "1", &models.User{Username: "operator"})

	var failures map[string]string
	require.Empty(t, identityReply(t, w, &failures))
	require.Equal(t, map[string]string{"invalid": "component_id is blank"}, failures)
	var saved []models.BuiltinPayload
	require.NoError(t, rt.Ctx.DB.Find(&saved).Error)
	require.Len(t, saved, 1)
	require.Equal(t, body[0].Content, saved[0].Content)
	require.Equal(t, "operator", saved[0].CreatedBy)
}

func TestBuiltinPayloadsGets_MergesFilteredDatabaseAndFileTemplates(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinPayload{})
	cache := builtinRound2Cache(t)
	cache.AddBuiltinPayload(&models.BuiltinPayload{
		Type: "collect", ComponentID: 7, Cate: "agent", Name: "CPU builtin", UUID: 100,
	})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BuiltinPayload{
		{Type: "collect", ComponentID: 7, Cate: "agent", Name: "CPU custom", UpdatedBy: "alice"},
		{Type: "collect", ComponentID: 7, Cate: "agent", Name: "CPU system", UpdatedBy: SYSTEM},
		{Type: "collect", ComponentID: 8, Cate: "agent", Name: "CPU foreign", UpdatedBy: "alice"},
		{Type: "dashboard", ComponentID: 7, Cate: "agent", Name: "CPU board", UpdatedBy: "alice"},
	}).Error)

	w := identityRequest(t, rt, rt.builtinPayloadsGets, nil, "1?type=collect&component_id=7&cate=agent&query=CPU", nil)

	var got []models.BuiltinPayload
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.ElementsMatch(t, []string{"CPU custom", "CPU builtin"}, []string{got[0].Name, got[1].Name})
}

func TestBuiltinPayloadCates_DeduplicatesDatabaseAndFileCategories(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinPayload{})
	cache := builtinRound2Cache(t)
	cache.AddBuiltinPayload(&models.BuiltinPayload{Type: "collect", ComponentID: 7, Cate: "agent", UUID: 100})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BuiltinPayload{
		{Type: "collect", ComponentID: 7, Cate: "agent", UpdatedBy: "alice"},
		{Type: "collect", ComponentID: 7, Cate: "custom", UpdatedBy: "alice"},
		{Type: "collect", ComponentID: 8, Cate: "excluded", UpdatedBy: "alice"},
	}).Error)

	w := identityRequest(t, rt, rt.builtinPayloadcatesGet, nil, "1?type=collect&component_id=7", nil)

	var got []string
	require.Empty(t, identityReply(t, w, &got))
	require.ElementsMatch(t, []string{"agent", "custom"}, got)
}

func TestBuiltinPayloadsPut_ExtractsMetadataAndPreservesCreation(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		content string
		tags    string
		note    string
	}{
		{name: "alert", typ: "alert", content: `{"name":"updated","append_tags":["env=prod"]}`, tags: "env=prod"},
		{
			name:    "dashboard",
			typ:     "dashboard",
			content: `{"name":"updated","tags":"prod","note":"guide"}`,
			tags:    "prod",
			note:    "guide",
		},
		{name: "collect", typ: "collect", content: "interval = 15"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.BuiltinPayload{})
			original := models.BuiltinPayload{
				ID:        1,
				Type:      tc.typ,
				Name:      "original",
				UUID:      123,
				CreatedBy: "alice",
				CreatedAt: 100,
			}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			form := models.BuiltinPayload{ID: 1, Type: tc.typ, Name: "updated", ComponentID: 7, Content: tc.content, UUID: 999}

			w := identityRequest(t, rt, rt.builtinPayloadsPut, form, "1", &models.User{Username: "operator"})

			require.Empty(t, identityReply(t, w, nil))
			var got models.BuiltinPayload
			require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
			require.Equal(t, "updated", got.Name)
			require.Equal(t, tc.tags, got.Tags)
			require.Equal(t, tc.note, got.Note)
			require.Equal(t, tc.content, got.Content)
			require.Equal(t, original.UUID, got.UUID)
			require.Equal(t, original.CreatedBy, got.CreatedBy)
			require.Equal(t, original.CreatedAt, got.CreatedAt)
			require.Equal(t, "operator", got.UpdatedBy)
		})
	}
}

func TestBuiltinPayloadsPut_InvalidContentDoesNotWrite(t *testing.T) {
	for _, typ := range []string{"alert", "dashboard", "collect"} {
		t.Run(typ, func(t *testing.T) {
			rt := identityRouter(t, &models.BuiltinPayload{})
			original := models.BuiltinPayload{ID: 1, Type: typ, Content: "original", Name: "original"}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)

			w := identityRequest(t, rt, rt.builtinPayloadsPut,
				models.BuiltinPayload{ID: 1, Type: typ, ComponentID: 7, Content: "{"}, "1", &models.User{Username: "operator"})

			require.Equal(t, http.StatusBadRequest, w.Code)
			var got models.BuiltinPayload
			require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
			require.Equal(t, original.Content, got.Content)
			require.Equal(t, original.Name, got.Name)
		})
	}
}

func TestBuiltinPayloadsGetByUUID_PrefersFileAndFallsBackToDatabase(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinPayload{})
	cache := builtinRound2Cache(t)
	cache.AddBuiltinPayload(&models.BuiltinPayload{UUID: 123, Name: "file", Type: "alert", ComponentID: 7})
	require.NoError(t,
		rt.Ctx.DB.Create(&[]models.BuiltinPayload{{UUID: 123, Name: "shadowed"}, {UUID: 124, Name: "custom"}}).Error)
	cases := []struct {
		name     string
		id       string
		expected string
	}{
		{name: "file first", id: "1?uuid=123", expected: "file"},
		{name: "database fallback", id: "1?uuid=124", expected: "custom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, rt.builtinPayloadsGetByUUID, nil, tc.id, nil)

			var got models.BuiltinPayload
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, tc.expected, got.Name)
		})
	}
}

func TestBuiltinPayloadsDel_PreservesUnselectedTemplates(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinPayload{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.BuiltinPayload{{ID: 1}, {ID: 2}}).Error)

	w := identityRequest(t, rt, rt.builtinPayloadsDel, idsForm{Ids: []int64{1, 404}}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var got []models.BuiltinPayload
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].ID)
}

func TestBuiltinPayloadsValidation_RejectsEmptyImportAndMissingObjects(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinPayload{})
	cases := []struct {
		name    string
		handler gin.HandlerFunc
		body    interface{}
		status  int
	}{
		{name: "empty import", handler: rt.builtinPayloadsAdd, body: []models.BuiltinPayload{}, status: 400},
		{name: "missing update", handler: rt.builtinPayloadsPut, body: models.BuiltinPayload{ID: 404}, status: 404},
		{name: "missing list type", handler: rt.builtinPayloadsGets, status: 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, tc.handler, tc.body, "1", &models.User{Username: "operator"})

			require.Equal(t, tc.status, w.Code)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.BuiltinPayload{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}
