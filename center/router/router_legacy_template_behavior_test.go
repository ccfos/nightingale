package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func legacyTemplateRouter(t *testing.T) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotifyTpl{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func legacyTemplateRequest(t *testing.T, handler gin.HandlerFunc, body interface{}, user *models.User) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("user", user) })
	engine.POST("/templates/:id", handler)
	req := httptest.NewRequest(http.MethodPost, "/templates/1", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func legacyTemplateReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
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

func TestNotifyTplUpdateContent_EnforcesCreatorOrAdmin(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		status int
	}{
		{name: "owner", user: models.User{Username: "alice"}, status: http.StatusOK},
		{name: "admin", user: models.User{Username: "admin", RolesLst: []string{models.AdminRole}}, status: http.StatusOK},
		{name: "outsider", user: models.User{Username: "bob"}, status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := legacyTemplateRouter(t)
			original := models.NotifyTpl{Id: 1, Name: "original", Channel: "custom", Content: "old", CreateBy: "alice"}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			replacement := models.NotifyTpl{Id: 1, Name: "spoofed", Channel: "different", Content: "{{$value}}"}

			w := legacyTemplateRequest(t, rt.notifyTplUpdateContent, replacement, &tc.user)

			require.Equal(t, tc.status, w.Code)
			var saved models.NotifyTpl
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, original.Name, saved.Name)
			require.Equal(t, original.Channel, saved.Channel)
			if tc.status == http.StatusOK {
				require.Empty(t, legacyTemplateReply(t, w, nil))
				require.Equal(t, replacement.Content, saved.Content)
				require.Equal(t, tc.user.Username, saved.UpdateBy)
			} else {
				require.Equal(t, original.Content, saved.Content)
			}
		})
	}
}

func TestNotifyTplAdd_RejectsInvalidTemplateBeforeWriting(t *testing.T) {
	cases := []struct {
		name     string
		template models.NotifyTpl
		message  string
	}{
		{name: "long channel", template: models.NotifyTpl{Channel: strings.Repeat("a", 33)}, message: "channel length"},
		{name: "long name", template: models.NotifyTpl{Name: strings.Repeat("a", 256)}, message: "name length"},
		{name: "invalid template", template: models.NotifyTpl{Content: "{{if}}"}, message: "verify illegal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := legacyTemplateRouter(t)

			w := legacyTemplateRequest(t, rt.notifyTplAdd, tc.template, &models.User{Username: "alice"})

			require.Contains(t, legacyTemplateReply(t, w, nil), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.NotifyTpl{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestNotifyTplAdd_TrimsChannelAndRejectsDuplicates(t *testing.T) {
	rt := legacyTemplateRouter(t)
	form := models.NotifyTpl{Name: "custom", Channel: " custom ", Content: "{{$value}}", CreateBy: "spoofed"}

	w := legacyTemplateRequest(t, rt.notifyTplAdd, form, &models.User{Username: "alice"})

	require.Empty(t, legacyTemplateReply(t, w, nil))
	var saved models.NotifyTpl
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, "custom", saved.Channel)
	require.Equal(t, "alice", saved.CreateBy)

	w = legacyTemplateRequest(t, rt.notifyTplAdd, form, &models.User{Username: "alice"})

	require.Contains(t, legacyTemplateReply(t, w, nil), "duplicate channel")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.NotifyTpl{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestNotifyTplDelete_ProtectsBuiltinsAndForeignOwners(t *testing.T) {
	cases := []struct {
		name    string
		channel string
		user    string
		status  int
		remains bool
	}{
		{name: "builtin", channel: models.Email, user: "alice", status: http.StatusOK, remains: true},
		{name: "foreign", channel: "custom", user: "bob", status: http.StatusForbidden, remains: true},
		{name: "owned custom", channel: "custom", user: "alice", status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := legacyTemplateRouter(t)
			require.NoError(t, rt.Ctx.DB.Create(&models.NotifyTpl{Id: 1, Channel: tc.channel, CreateBy: "alice"}).Error)

			w := legacyTemplateRequest(t, rt.notifyTplDel, nil, &models.User{Username: tc.user})

			require.Equal(t, tc.status, w.Code)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.NotifyTpl{}).Count(&count).Error)
			require.Equal(t, tc.remains, count == 1)
		})
	}
}

func TestNotifyTplUpdate_RejectsDuplicateNameWithoutWriting(t *testing.T) {
	rt := legacyTemplateRouter(t)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.NotifyTpl{
		{Id: 1, Name: "original", Channel: "one", CreateBy: "alice"}, {Id: 2, Name: "taken", Channel: "two"},
	}).Error)

	w := legacyTemplateRequest(t, rt.notifyTplUpdate,
		models.NotifyTpl{Id: 1, Name: "taken", Channel: "one"}, &models.User{Username: "alice"})

	require.Contains(t, legacyTemplateReply(t, w, nil), "duplicate channel or name")
	var saved models.NotifyTpl
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "original", saved.Name)
}

func TestNotifyTplPreview_RendersEventFields(t *testing.T) {
	rt := &Router{}

	w := legacyTemplateRequest(t, rt.notifyTplPreview,
		models.NotifyTpl{Channel: "custom", Content: `severity={{.Severity}};value={{$value}}`}, nil)

	var output string
	require.Empty(t, legacyTemplateReply(t, w, &output))
	// The preview renders the fixed EVENT_EXAMPLE shipped with the center.
	require.Equal(t, "severity=0;value=2273533952", output)
}

func TestNotifyTplPreview_ReturnsTemplateExecutionError(t *testing.T) {
	rt := &Router{}

	w := legacyTemplateRequest(t, rt.notifyTplPreview, models.NotifyTpl{
		Channel: "custom",
		Content: `{{.Nonexistent}}`,
	}, nil)

	var output string
	require.Empty(t, legacyTemplateReply(t, w, &output))
	require.Contains(t, output, "can't evaluate field Nonexistent")
}
