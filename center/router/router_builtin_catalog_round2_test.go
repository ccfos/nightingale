package router

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func builtinCatalogRound2Dir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "router-builtin-catalog-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	fixtures := map[string]string{
		"Linux/dashboards/cpu.json":    `{"name":"CPU","tags":"linux","configs":{"panels":[1]}}`,
		"Linux/dashboards/broken.json": `{`,
		"Linux/dashboards/notes.txt":   `not a dashboard`,
		"Linux/alerts/cpu.json":        `[{"name":"CPU high","severity":2},{"name":"CPU low","severity":3}]`,
		"Linux/alerts/broken.json":     `{`,
		"Linux/icon/linux.svg":         `<svg/>`,
	}
	for name, content := range fixtures {
		target := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0700))
		require.NoError(t, os.WriteFile(target, []byte(content), 0600))
	}
	return dir
}

func TestBuiltinBoardCateGets_ListsValidSummariesAndFavorites(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinCate{})
	rt.Center.BuiltinIntegrationsDir = builtinCatalogRound2Dir(t)
	require.NoError(t, rt.Ctx.DB.Create(&models.BuiltinCate{Name: "Linux", UserId: 1}).Error)

	w := identityRequest(t, rt, rt.builtinBoardCateGets, nil, "1", &models.User{Id: 1})

	var got []BoardCate
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "Linux", got[0].Name)
	require.True(t, got[0].Favorite)
	require.Equal(t, "/api/n9e/integrations/icon/Linux/linux.svg", got[0].IconUrl)
	require.Equal(t, []Payload{{Cate: "Linux", Fname: "cpu.json", Name: "CPU", Tags: "linux", Configs: ""}}, got[0].Boards)
}

func TestBuiltinBoardDetailGets_LoadsSelectedDashboardConfiguration(t *testing.T) {
	rt := &Router{}
	rt.Center.BuiltinIntegrationsDir = builtinCatalogRound2Dir(t)

	w := identityRequest(t, rt, rt.builtinBoardDetailGets, Payload{Cate: "Linux", Fname: "cpu.json"}, "1", nil)

	var got Payload
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "CPU", got.Name)
	require.Equal(t, "linux", got.Tags)
	require.Equal(t, map[string]interface{}{"panels": []interface{}{float64(1)}}, got.Configs)
}

func TestBuiltinBoardGets_ListsOnlyJSONNames(t *testing.T) {
	rt := &Router{}
	rt.Center.BuiltinIntegrationsDir = builtinCatalogRound2Dir(t)

	w := identityRequest(t, rt, rt.builtinBoardGets, nil, "1", nil)

	var got []string
	require.Empty(t, identityReply(t, w, &got))
	// This endpoint lists filenames; JSON validation belongs to the detail endpoint.
	require.ElementsMatch(t, []string{"cpu", "broken"}, got)
}

func TestBuiltinAlertCateGets_SkipsMalformedRulesAndIncludesFavorites(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinCate{})
	rt.Center.BuiltinIntegrationsDir = builtinCatalogRound2Dir(t)
	require.NoError(t, rt.Ctx.DB.Create(&models.BuiltinCate{Name: "Linux", UserId: 1}).Error)

	w := identityRequest(t, rt, rt.builtinAlertCateGets, nil, "1", &models.User{Id: 1})

	var got []AlertCate
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "Linux", got[0].Name)
	require.True(t, got[0].Favorite)
	require.Equal(t, "/api/n9e/integrations/icon/Linux/linux.svg", got[0].IconUrl)
	require.Len(t, got[0].AlertRules, 2)
	require.Equal(t, "CPU high", got[0].AlertRules[0].Name)
	require.Equal(t, 2, got[0].AlertRules[0].Severity)
	require.Equal(t, "CPU low", got[0].AlertRules[1].Name)
}

func TestBuiltinAlertRules_GroupsRulesBySourceFile(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinCate{})
	rt.Center.BuiltinIntegrationsDir = builtinCatalogRound2Dir(t)
	require.NoError(t, rt.Ctx.DB.Create(&models.BuiltinCate{Name: "Linux", UserId: 1}).Error)

	w := identityRequest(t, rt, rt.builtinAlertRules, nil, "1", &models.User{Id: 1})

	var got []builtinAlertRulesList
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.True(t, got[0].Favorite)
	require.Equal(t, "/api/n9e/integrations/icon/Linux/linux.svg", got[0].IconUrl)
	require.Len(t, got[0].AlertRules, 1)
	require.Len(t, got[0].AlertRules["cpu"], 2)
	require.Equal(t, "CPU high", got[0].AlertRules["cpu"][0].Name)
}

func TestBuiltinBoardGet_ReturnsRawJSONOrMissingNameError(t *testing.T) {
	rt := &Router{}
	rt.Center.BuiltinIntegrationsDir = builtinCatalogRound2Dir(t)
	for _, name := range []string{"cpu", "missing"} {
		t.Run(name, func(t *testing.T) {
			handler := func(c *gin.Context) {
				c.Params = append(c.Params, gin.Param{Key: "name", Value: name})
				rt.builtinBoardGet(c)
			}

			w := identityRequest(t, rt, handler, nil, "1", nil)

			if name == "missing" {
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Contains(t, w.Body.String(), "missing not found")
			} else {
				var got string
				require.Empty(t, identityReply(t, w, &got))
				require.JSONEq(t, `{"name":"CPU","tags":"linux","configs":{"panels":[1]}}`, got)
			}
		})
	}
}

func TestBuiltinCateFavorites_UsesAuthenticatedUserAndDeletesOnlyTheirFavorite(t *testing.T) {
	rt := identityRouter(t, &models.BuiltinCate{})
	require.NoError(t, rt.Ctx.DB.Create(&models.BuiltinCate{Name: "Linux", UserId: 2}).Error)

	added := identityRequest(t, rt, rt.builtinCateFavoriteAdd,
		models.BuiltinCate{Name: "Linux", UserId: 999}, "1", &models.User{Id: 1})

	require.Empty(t, identityReply(t, added, nil))
	favorites, err := models.BuiltinCateGetByUserId(rt.Ctx, 1)
	require.NoError(t, err)
	require.Contains(t, favorites, "Linux")
	handler := func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "name", Value: "Linux"})
		rt.builtinCateFavoriteDel(c)
	}

	deleted := identityRequest(t, rt, handler, nil, "1", &models.User{Id: 1})

	require.Empty(t, identityReply(t, deleted, nil))
	favorites, err = models.BuiltinCateGetByUserId(rt.Ctx, 1)
	require.NoError(t, err)
	require.Empty(t, favorites)
	favorites, err = models.BuiltinCateGetByUserId(rt.Ctx, 2)
	require.NoError(t, err)
	require.Contains(t, favorites, "Linux")
}
