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

func channelBehaviorRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(append([]interface{}{&models.NotifyChannelConfig{}}, tables...)...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func validBehaviorChannel() models.NotifyChannelConfig {
	return models.NotifyChannelConfig{
		Name: "callback", Ident: "callback", Enable: true, RequestType: "http",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
			URL: "http://127.0.0.1/unused", Method: "POST", Headers: map[string]string{"Authorization": "secret"},
		}},
	}
}

func channelBehaviorRequest(t *testing.T, handler gin.HandlerFunc, body interface{}, query string) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) {
		c.Set("user", &models.User{Username: "editor", RolesLst: []string{models.AdminRole}})
	})
	engine.POST("/channels/:id", handler)
	req := httptest.NewRequest(http.MethodPost, "/channels/1"+query, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func channelBehaviorReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
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

func TestNotifyChannelsAdd_ValidatesEntireBatchBeforeWriting(t *testing.T) {
	rt := channelBehaviorRouter(t)
	valid := validBehaviorChannel()
	invalid := validBehaviorChannel()
	invalid.Name = ""

	w := channelBehaviorRequest(t, rt.notifyChannelsAdd, []models.NotifyChannelConfig{valid, invalid}, "")

	require.Contains(t, channelBehaviorReply(t, w, nil), "channel name cannot be empty")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.NotifyChannelConfig{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestNotifyChannelsAdd_RejectsExistingNameWithoutWriting(t *testing.T) {
	rt := channelBehaviorRouter(t)
	channel := validBehaviorChannel()
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)

	w := channelBehaviorRequest(t, rt.notifyChannelsAdd, []models.NotifyChannelConfig{validBehaviorChannel()}, "")

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "name already exists")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.NotifyChannelConfig{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestNotifyChannelsAdd_RecordsAuthenticatedAuthor(t *testing.T) {
	rt := channelBehaviorRouter(t)
	channel := validBehaviorChannel()
	channel.CreateBy = "spoofed"

	w := channelBehaviorRequest(t, rt.notifyChannelsAdd, []models.NotifyChannelConfig{channel}, "")

	var ids []int64
	require.Empty(t, channelBehaviorReply(t, w, &ids))
	require.Len(t, ids, 1)
	var saved models.NotifyChannelConfig
	require.NoError(t, rt.Ctx.DB.First(&saved, ids[0]).Error)
	require.Equal(t, "editor", saved.CreateBy)
	require.Equal(t, "editor", saved.UpdateBy)
	require.Equal(t, channel.RequestConfig, saved.RequestConfig)
}

func TestNotifyChannelsDelete_ProtectsReferencedChannels(t *testing.T) {
	rt := channelBehaviorRouter(t, &models.NotifyRule{})
	channel := validBehaviorChannel()
	channel.ID = 1
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)
	rule := models.NotifyRule{ID: 9, Name: "reference", NotifyConfigs: []models.NotifyConfig{{ChannelID: 1}}}
	require.NoError(t, rt.Ctx.DB.Create(&rule).Error)

	w := channelBehaviorRequest(t, rt.notifyChannelsDel, idsForm{Ids: []int64{1}}, "")

	require.Contains(t, channelBehaviorReply(t, w, nil), "used by notify rule: [9]")
	var saved models.NotifyChannelConfig
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
}

func TestNotifyChannelsDelete_RemovesUnusedChannels(t *testing.T) {
	rt := channelBehaviorRouter(t, &models.NotifyRule{})
	channel := validBehaviorChannel()
	channel.ID = 1
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)

	w := channelBehaviorRequest(t, rt.notifyChannelsDel, idsForm{Ids: []int64{1}}, "")

	require.Empty(t, channelBehaviorReply(t, w, nil))
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.NotifyChannelConfig{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestNotifyChannelPut_PreservesCreationMetadata(t *testing.T) {
	rt := channelBehaviorRouter(t)
	original := validBehaviorChannel()
	original.ID = 1
	original.CreateBy = "creator"
	original.CreateAt = 123
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	replacement := validBehaviorChannel()
	replacement.ID = 1
	replacement.Name = "updated"
	replacement.CreateBy = "spoofed"
	replacement.Enable = false

	w := channelBehaviorRequest(t, rt.notifyChannelPut, replacement, "")

	require.Empty(t, channelBehaviorReply(t, w, nil))
	var saved models.NotifyChannelConfig
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, original.CreateBy, saved.CreateBy)
	require.Equal(t, original.CreateAt, saved.CreateAt)
	require.Equal(t, "editor", saved.UpdateBy)
	require.Equal(t, replacement.Name, saved.Name)
	require.False(t, saved.Enable)
}

func TestNotifyChannelsGetForNormalUser_OmitsRequestSecrets(t *testing.T) {
	rt := channelBehaviorRouter(t)
	channel := validBehaviorChannel()
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)

	w := channelBehaviorRequest(t, rt.notifyChannelsGetForNormalUser, nil, "")

	var channels []models.NotifyChannelConfig
	require.Empty(t, channelBehaviorReply(t, w, &channels))
	require.Len(t, channels, 1)
	require.Equal(t, channel.Ident, channels[0].Ident)
	require.Nil(t, channels[0].RequestConfig)
	require.NotContains(t, w.Body.String(), "secret")
	require.NotContains(t, w.Body.String(), "Authorization")
}

func TestNotifyChannelIdentsGet_DeduplicatesAndSorts(t *testing.T) {
	rt := channelBehaviorRouter(t)
	channels := []models.NotifyChannelConfig{{Name: "one", Ident: "sms"}, {Name: "two", Ident: "email"},
		{Name: "three", Ident: "sms"}, {Name: "empty"}}
	require.NoError(t, rt.Ctx.DB.Create(&channels).Error)

	w := channelBehaviorRequest(t, rt.notifyChannelIdentsGet, nil, "")

	var got []string
	require.Empty(t, channelBehaviorReply(t, w, &got))
	require.Equal(t, []string{"email", "sms"}, got)
}

func TestNotifyChannelGetBy_OmitsParameterAndRequestConfiguration(t *testing.T) {
	rt := channelBehaviorRouter(t)
	channel := validBehaviorChannel()
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)

	w := channelBehaviorRequest(t, rt.notifyChannelGetBy, nil, "?ident=callback")

	var got models.NotifyChannelConfig
	require.Empty(t, channelBehaviorReply(t, w, &got))
	require.Equal(t, channel.ID, got.ID)
	require.Equal(t, &models.NotifyParamConfig{}, got.ParamConfig)
	require.Equal(t, &models.RequestConfig{}, got.RequestConfig)
	require.NotContains(t, w.Body.String(), "secret")
}

func TestNotifyChannelGet_ReturnsNotFound(t *testing.T) {
	rt := channelBehaviorRouter(t)

	w := channelBehaviorRequest(t, rt.notifyChannelGet, nil, "")

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "notify channel not found")
}
