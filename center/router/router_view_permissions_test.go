package router

import (
	"bytes"
	"encoding/json"
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

func viewPermissionRouter(t *testing.T, model interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(model))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func viewPermissionRequest(t *testing.T, handler gin.HandlerFunc, body interface{}, user *models.User) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("user", user); c.Set("userid", user.Id) })
	engine.POST("/views", handler)
	req := httptest.NewRequest(http.MethodPost, "/views", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func viewPermissionReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
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

func TestMetricViewAdd_ForcesPrivateVisibilityForNonAdmin(t *testing.T) {
	rt := viewPermissionRouter(t, &models.MetricView{})

	w := viewPermissionRequest(t, rt.metricViewAdd,
		models.MetricView{Id: 99, Name: "mine", Configs: "{}", Cate: 0, CreateBy: 2}, &models.User{Id: 1})

	var got models.MetricView
	require.Empty(t, viewPermissionReply(t, w, &got))
	require.NotEqual(t, int64(99), got.Id)
	require.Equal(t, int64(1), got.CreateBy)
	require.Equal(t, 1, got.Cate)
}

func TestMetricViewPut_RejectsForeignOwnerWithoutWriting(t *testing.T) {
	rt := viewPermissionRouter(t, &models.MetricView{})
	require.NoError(t, rt.Ctx.DB.Create(&models.MetricView{Id: 1, Name: "original", Configs: "{}", CreateBy: 2}).Error)

	w := viewPermissionRequest(t, rt.metricViewPut, models.MetricView{
		Id:      1,
		Name:    "changed",
		Configs: "new",
	}, &models.User{
		Id: 1,
	})

	require.Equal(t, http.StatusForbidden, w.Code)
	var saved models.MetricView
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "original", saved.Name)
	require.Equal(t, "{}", saved.Configs)
}

func TestMetricViewPut_OwnerCannotMakeViewPublic(t *testing.T) {
	rt := viewPermissionRouter(t, &models.MetricView{})
	require.NoError(t, rt.Ctx.DB.Create(&models.MetricView{
		Id:       1,
		Name:     "original",
		Configs:  "{}",
		CreateBy: 1,
		Cate:     1,
	}).Error)

	w := viewPermissionRequest(t, rt.metricViewPut,
		models.MetricView{Id: 1, Name: "changed", Configs: "updated", Cate: 0}, &models.User{Id: 1})

	require.Empty(t, viewPermissionReply(t, w, nil))
	var saved models.MetricView
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "changed", saved.Name)
	require.Equal(t, "updated", saved.Configs)
	require.Equal(t, 1, saved.Cate)
}

func TestMetricViewDelete_DeletesOnlyOwnedViews(t *testing.T) {
	rt := viewPermissionRouter(t, &models.MetricView{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.MetricView{{Id: 1, CreateBy: 1}, {Id: 2, CreateBy: 2}}).Error)

	w := viewPermissionRequest(t, rt.metricViewDel, idsForm{Ids: []int64{1, 2}}, &models.User{Id: 1})

	require.Empty(t, viewPermissionReply(t, w, nil))
	var got []models.MetricView
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].Id)
}

func TestMetricViewGets_ReturnsPublicAndOwnedViews(t *testing.T) {
	rt := viewPermissionRouter(t, &models.MetricView{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.MetricView{
		{Id: 1, Name: "mine", Cate: 1, CreateBy: 1}, {Id: 2, Name: "public", Cate: 0, CreateBy: 2},
		{Id: 3, Name: "foreign", Cate: 1, CreateBy: 2},
	}).Error)

	w := viewPermissionRequest(t, rt.metricViewGets, nil, &models.User{Id: 1})

	var got []models.MetricView
	require.Empty(t, viewPermissionReply(t, w, &got))
	require.Len(t, got, 2)
	require.Equal(t, "public", got[0].Name)
	require.Equal(t, "mine", got[1].Name)
}

func TestAlertAggrViewAdd_ValidatesRuleAndRecordsCreator(t *testing.T) {
	rt := viewPermissionRouter(t, &models.AlertAggrView{})

	w := viewPermissionRequest(t, rt.alertAggrViewAdd,
		models.AlertAggrView{Name: "by service", Rule: "tagkey:service", CreateBy: 2}, &models.User{Id: 1})

	var got models.AlertAggrView
	require.Empty(t, viewPermissionReply(t, w, &got))
	require.Equal(t, int64(1), got.CreateBy)
	require.Equal(t, "tagkey:service", got.Rule)
	require.Equal(t, 1, got.Cate)
}

func TestAlertAggrViewPut_RejectsForeignOwnerWithoutWriting(t *testing.T) {
	rt := viewPermissionRouter(t, &models.AlertAggrView{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertAggrView{
		Id:       1,
		Name:     "original",
		Rule:     "field:severity",
		CreateBy: 2,
	}).Error)

	w := viewPermissionRequest(t, rt.alertAggrViewPut,
		models.AlertAggrView{Id: 1, Name: "changed", Rule: "tagkey:service"}, &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	var saved models.AlertAggrView
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "original", saved.Name)
	require.Equal(t, "field:severity", saved.Rule)
}

func TestAlertAggrViewPut_OwnerUpdatesPrivateRule(t *testing.T) {
	rt := viewPermissionRouter(t, &models.AlertAggrView{})
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertAggrView{
		Id:       1,
		Name:     "original",
		Rule:     "field:severity",
		CreateBy: 1,
	}).Error)

	w := viewPermissionRequest(t, rt.alertAggrViewPut,
		models.AlertAggrView{Id: 1, Name: "changed", Rule: "tagkey:service", Cate: 0}, &models.User{Id: 1})

	require.Empty(t, viewPermissionReply(t, w, nil))
	var saved models.AlertAggrView
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "changed", saved.Name)
	require.Equal(t, "tagkey:service", saved.Rule)
	require.Equal(t, 1, saved.Cate)
}

func TestAlertAggrViewDelete_DeletesOnlyOwnedViews(t *testing.T) {
	rt := viewPermissionRouter(t, &models.AlertAggrView{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertAggrView{{Id: 1, CreateBy: 1}, {Id: 2, CreateBy: 2}}).Error)

	w := viewPermissionRequest(t, rt.alertAggrViewDel, idsForm{Ids: []int64{1, 2}}, &models.User{Id: 1})

	require.Empty(t, viewPermissionReply(t, w, nil))
	var got []models.AlertAggrView
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].Id)
}
