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

func notificationOwnerRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func seedNotificationMembership(t *testing.T, rt *Router) {
	t.Helper()
	db := rt.Ctx.DB
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
}

func notificationOwnerRequest(t *testing.T, handler gin.HandlerFunc, user *models.User,
	body interface{}, query string) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("user", user) })
	engine.POST("/objects/:id", handler)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/objects/1"+query, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Language", "en_US")
	engine.ServeHTTP(w, req)
	return w
}

func notificationOwnerReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
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

func TestNotifyRulesAdd_RequiresMembershipAndCleansDisplayFields(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		status int
	}{
		{name: "member", user: models.User{Id: 1, Username: "member"}, status: http.StatusOK},
		{name: "outsider", user: models.User{Id: 2, Username: "outsider"}, status: http.StatusForbidden},
		{name: "admin", user: models.User{Username: "admin", RolesLst: []string{models.AdminRole}}, status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := notificationOwnerRouter(t,
				&models.NotifyRule{},
				&models.UserGroupMember{},
			)
			seedNotificationMembership(t, rt)
			rule := models.NotifyRule{
				Name: "oncall", Enable: true, UserGroupIds: []int64{10}, CreateBy: "spoofed",
				NotifyConfigs: []models.NotifyConfig{{
					ChannelID: 5, TemplateID: 9, Severities: []int{2}, ChannelIdent: "spoofed",
					UserNames: []string{"display only"}, UserGroupNames: []string{"display only"},
				}},
			}

			w := notificationOwnerRequest(t, rt.notifyRulesAdd, &tc.user, []models.NotifyRule{rule}, "")

			require.Equal(t, tc.status, w.Code, w.Body.String())
			var saved []models.NotifyRule
			require.NoError(t, rt.Ctx.DB.Find(&saved).Error)
			if tc.status != http.StatusOK {
				require.Empty(t, saved)
				require.Contains(t, w.Body.String(), "forbidden")
				return
			}
			require.Empty(t, notificationOwnerReply(t, w, nil))
			require.Len(t, saved, 1)
			require.Equal(t, tc.user.Username, saved[0].CreateBy)
			require.Equal(t, tc.user.Username, saved[0].UpdateBy)
			require.Len(t, saved[0].NotifyConfigs, 1)
			require.Equal(t, int64(5), saved[0].NotifyConfigs[0].ChannelID)
			require.Empty(t, saved[0].NotifyConfigs[0].ChannelIdent)
			require.Empty(t, saved[0].NotifyConfigs[0].UserNames)
			require.Empty(t, saved[0].NotifyConfigs[0].UserGroupNames)
		})
	}
}

func TestNotifyRulesAdd_RejectsInvalidNotificationConfig(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{name: "empty", body: `[]`, message: "input json is empty"},
		{name: "blank name", body: `[{"user_group_ids":[10]}]`, message: "name cannot be empty"},
		{
			name: "invalid channel", body: `[{"name":"bad","user_group_ids":[10],"notify_configs":[{"channel_id":0}]}]`,
			message: "invalid channel id",
		},
		{
			name:    "invalid severity",
			body:    `[{"name":"bad","user_group_ids":[10],"notify_configs":[{"channel_id":1,"severities":[4]}]}]`,
			message: "invalid severity level",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := notificationOwnerRouter(t,
				&models.NotifyRule{},
				&models.UserGroupMember{},
			)
			seedNotificationMembership(t, rt)

			w := notificationOwnerRequest(t, rt.notifyRulesAdd, &models.User{Id: 1}, json.RawMessage(tc.body), "")

			require.Contains(t, w.Body.String(), tc.message)
			if tc.name == "empty" {
				require.Equal(t, http.StatusBadRequest, w.Code)
			} else {
				require.NotEmpty(t, notificationOwnerReply(t, w, nil))
			}
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.NotifyRule{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestNotifyRulesDelete_ChecksAllOwnersBeforeDeleting(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.NotifyRule{},
		&models.UserGroupMember{},
	)
	seedNotificationMembership(t, rt)
	rules := []models.NotifyRule{
		{ID: 1, Name: "own", UserGroupIds: []int64{10}},
		{ID: 2, Name: "other", UserGroupIds: []int64{20}},
	}
	require.NoError(t, rt.Ctx.DB.Create(&rules).Error)

	w := notificationOwnerRequest(t, rt.notifyRulesDel, &models.User{Id: 1}, idsForm{Ids: []int64{1, 2}}, "")

	require.Equal(t, http.StatusForbidden, w.Code)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.NotifyRule{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestNotifyRulesDelete_AllowsOwnerAndAdmin(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(fmt.Sprintf("admin=%t", admin), func(t *testing.T) {
			rt := notificationOwnerRouter(t,
				&models.NotifyRule{},
				&models.UserGroupMember{},
			)
			seedNotificationMembership(t, rt)
			user := &models.User{Id: 1}
			gid := int64(10)
			if admin {
				user.RolesLst = []string{models.AdminRole}
				gid = 20
			}
			require.NoError(t, rt.Ctx.DB.Create(&models.NotifyRule{ID: 1, Name: "delete", UserGroupIds: []int64{gid}}).Error)

			w := notificationOwnerRequest(t, rt.notifyRulesDel, user, idsForm{Ids: []int64{1}}, "")

			require.Empty(t, notificationOwnerReply(t, w, nil))
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.NotifyRule{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestNotifyRulePut_EnforcesExistingOwnershipAndPreservesCreator(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(fmt.Sprintf("allowed=%t", allowed), func(t *testing.T) {
			rt := notificationOwnerRouter(t,
				&models.NotifyRule{},
				&models.UserGroupMember{},
			)
			seedNotificationMembership(t, rt)
			original := models.NotifyRule{ID: 1, Name: "original", UserGroupIds: []int64{10}, CreateBy: "creator", CreateAt: 123}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			user := &models.User{Id: 1, Username: "editor"}
			if !allowed {
				user.Id = 2
			}
			replacement := models.NotifyRule{Name: "updated", UserGroupIds: []int64{10}, CreateBy: "spoofed"}

			w := notificationOwnerRequest(t, rt.notifyRulePut, user, replacement, "")

			var saved models.NotifyRule
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, original.CreateBy, saved.CreateBy)
			require.Equal(t, original.CreateAt, saved.CreateAt)
			if allowed {
				require.Empty(t, notificationOwnerReply(t, w, nil))
				require.Equal(t, "updated", saved.Name)
				require.Equal(t, "editor", saved.UpdateBy)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Equal(t, original.Name, saved.Name)
			}
		})
	}
}

func TestNotifyRulesGet_FiltersByMembershipAndFillsChannelIdent(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.NotifyRule{},
		&models.UserGroupMember{},
		&models.NotifyChannelConfig{},
		&models.User{},
	)
	seedNotificationMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.NotifyChannelConfig{ID: 5, Ident: "email"}).Error)
	rules := []models.NotifyRule{
		{Name: "own", UserGroupIds: []int64{10}, NotifyConfigs: []models.NotifyConfig{{ChannelID: 5}}},
		{Name: "other", UserGroupIds: []int64{20}},
	}
	require.NoError(t, rt.Ctx.DB.Create(&rules).Error)

	w := notificationOwnerRequest(t, rt.notifyRulesGet, &models.User{Id: 1}, nil, "")

	var got []models.NotifyRule
	require.Empty(t, notificationOwnerReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "own", got[0].Name)
	require.Equal(t, "email", got[0].NotifyConfigs[0].ChannelIdent)
}

func TestNotifyRuleGet_RejectsOtherTeamsAndMissingRules(t *testing.T) {
	for _, exists := range []bool{true, false} {
		t.Run(fmt.Sprintf("exists=%t", exists), func(t *testing.T) {
			rt := notificationOwnerRouter(t,
				&models.NotifyRule{},
				&models.UserGroupMember{},
			)
			seedNotificationMembership(t, rt)
			if exists {
				require.NoError(t, rt.Ctx.DB.Create(&models.NotifyRule{ID: 1, Name: "private", UserGroupIds: []int64{20}}).Error)
			}

			w := notificationOwnerRequest(t, rt.notifyRuleGet, &models.User{Id: 1}, nil, "")

			if exists {
				require.Equal(t, http.StatusForbidden, w.Code)
			} else {
				require.Equal(t, http.StatusNotFound, w.Code)
				require.Contains(t, w.Body.String(), "notify rule not found")
			}
		})
	}
}

func TestMessageTemplatesAdd_GeneratesIdentAndRequestLanguage(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.MessageTemplate{},
		&models.UserGroupMember{},
	)
	seedNotificationMembership(t, rt)
	template := models.MessageTemplate{
		Name: "custom", Ident: "caller-chosen", Lang: "zh_CN", Private: 1,
		UserGroupIds: []int64{10}, Content: map[string]string{"body": "{{.events}}"},
	}

	w := notificationOwnerRequest(t, rt.messageTemplatesAdd, &models.User{Id: 1, Username: "author"},
		[]models.MessageTemplate{template}, "")

	var ids []int64
	require.Empty(t, notificationOwnerReply(t, w, &ids))
	require.Len(t, ids, 1)
	var saved models.MessageTemplate
	require.NoError(t, rt.Ctx.DB.First(&saved, ids[0]).Error)
	require.NotEmpty(t, saved.Ident)
	require.NotEqual(t, template.Ident, saved.Ident)
	require.Equal(t, "en", saved.Lang)
	require.Equal(t, "author", saved.CreateBy)
	require.Equal(t, template.Content, saved.Content)
}

func TestMessageTemplatesAdd_RejectsForeignTeamWithoutWriting(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.MessageTemplate{},
		&models.UserGroupMember{},
	)
	seedNotificationMembership(t, rt)
	template := models.MessageTemplate{Name: "private", Private: 1, UserGroupIds: []int64{20}}

	w := notificationOwnerRequest(t, rt.messageTemplatesAdd, &models.User{Id: 1}, []models.MessageTemplate{template}, "")

	require.Equal(t, http.StatusForbidden, w.Code)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.MessageTemplate{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestMessageTemplatesAdd_ValidatesEntireBatchBeforeWriting(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.MessageTemplate{},
		&models.UserGroupMember{},
	)
	seedNotificationMembership(t, rt)
	templates := []models.MessageTemplate{{Name: "valid", UserGroupIds: []int64{10}}, {Name: ""}}

	w := notificationOwnerRequest(t, rt.messageTemplatesAdd, &models.User{Id: 1}, templates, "")

	require.Contains(t, notificationOwnerReply(t, w, nil), "template name cannot be empty")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.MessageTemplate{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestMessageTemplateGet_EnforcesPrivacy(t *testing.T) {
	cases := []struct {
		name    string
		private int
		user    models.User
		status  int
	}{
		{name: "public outsider", private: 0, user: models.User{Id: 2}, status: http.StatusOK},
		{name: "private member", private: 1, user: models.User{Id: 1}, status: http.StatusOK},
		{name: "private outsider", private: 1, user: models.User{Id: 2}, status: http.StatusForbidden},
		{name: "private admin", private: 1, user: models.User{RolesLst: []string{models.AdminRole}}, status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := notificationOwnerRouter(t,
				&models.MessageTemplate{},
				&models.UserGroupMember{},
			)
			seedNotificationMembership(t, rt)
			template := models.MessageTemplate{ID: 1, Name: "template", Private: tc.private, UserGroupIds: []int64{10}}
			require.NoError(t, rt.Ctx.DB.Create(&template).Error)

			w := notificationOwnerRequest(t, rt.messageTemplateGet, &tc.user, nil, "")

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				var got models.MessageTemplate
				require.Empty(t, notificationOwnerReply(t, w, &got))
				require.Equal(t, template.ID, got.ID)
				require.Equal(t, template.Private, got.Private)
			}
		})
	}
}

func TestMessageTemplatePut_PreservesLanguageAndCreator(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.MessageTemplate{},
		&models.UserGroupMember{},
	)
	seedNotificationMembership(t, rt)
	original := models.MessageTemplate{
		ID: 1, Name: "original", Ident: "immutable", Lang: "zh_CN", UserGroupIds: []int64{
			10,
		}, CreateBy: "creator", CreateAt: 123,
	}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	changed := models.MessageTemplate{
		Name:  "updated",
		Ident: "immutable",
		Lang:  "en_US",
		Content: map[string]string{
			"body": "new",
		},
	}

	w := notificationOwnerRequest(t, rt.messageTemplatePut, &models.User{Id: 1, Username: "editor"}, changed, "")

	require.Empty(t, notificationOwnerReply(t, w, nil))
	var saved models.MessageTemplate
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, original.Lang, saved.Lang)
	require.Equal(t, original.CreateBy, saved.CreateBy)
	require.Equal(t, original.CreateAt, saved.CreateAt)
	require.Equal(t, "editor", saved.UpdateBy)
	require.Equal(t, changed.Content, saved.Content)
}

func TestMessageTemplatePut_RejectsForeignOwnerAndChangedIdent(t *testing.T) {
	cases := []struct {
		name    string
		uid     int64
		ident   string
		status  int
		message string
	}{
		{name: "foreign owner", uid: 2, ident: "immutable", status: http.StatusForbidden, message: "forbidden"},
		{name: "changed ident", uid: 1, ident: "different", status: http.StatusOK, message: "cannot update ident"},
		{name: "duplicate ident", uid: 1, ident: "taken", status: http.StatusBadRequest, message: "ident already exists"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := notificationOwnerRouter(t,
				&models.MessageTemplate{},
				&models.UserGroupMember{},
			)
			seedNotificationMembership(t, rt)
			templates := []models.MessageTemplate{
				{ID: 1, Name: "original", Ident: "immutable", UserGroupIds: []int64{10}},
				{ID: 2, Name: "other", Ident: "taken"},
			}
			require.NoError(t, rt.Ctx.DB.Create(&templates).Error)

			w := notificationOwnerRequest(t, rt.messageTemplatePut, &models.User{Id: tc.uid},
				models.MessageTemplate{Name: "changed", Ident: tc.ident}, "")

			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			var saved models.MessageTemplate
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, "original", saved.Name)
			require.Equal(t, "immutable", saved.Ident)
		})
	}
}

func TestMessageTemplatesDelete_RejectsReferencedTemplates(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.MessageTemplate{},
		&models.NotifyRule{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.MessageTemplate{ID: 1, Name: "used"}).Error)
	rule := models.NotifyRule{ID: 9, Name: "reference", NotifyConfigs: []models.NotifyConfig{{TemplateID: 1}}}
	require.NoError(t, rt.Ctx.DB.Create(&rule).Error)

	w := notificationOwnerRequest(t, rt.messageTemplatesDel, &models.User{RolesLst: []string{models.AdminRole}},
		idsForm{Ids: []int64{1}}, "")

	require.Contains(t, notificationOwnerReply(t, w, nil), "used by notify rule: [9]")
	var saved models.MessageTemplate
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
}

func TestMessageTemplatesDelete_RequiresOwnershipForEntireBatch(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.MessageTemplate{},
		&models.UserGroupMember{},
		&models.NotifyRule{},
	)
	seedNotificationMembership(t, rt)
	templates := []models.MessageTemplate{
		{ID: 1, Name: "own", UserGroupIds: []int64{10}},
		{ID: 2, Name: "other", UserGroupIds: []int64{20}},
	}
	require.NoError(t, rt.Ctx.DB.Create(&templates).Error)

	w := notificationOwnerRequest(t, rt.messageTemplatesDel, &models.User{Id: 1}, idsForm{Ids: []int64{1, 2}}, "")

	require.Equal(t, http.StatusForbidden, w.Code)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.MessageTemplate{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestMessageTemplatesDelete_AllowsOwner(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.MessageTemplate{},
		&models.UserGroupMember{},
		&models.NotifyRule{},
	)
	seedNotificationMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.MessageTemplate{ID: 1, Name: "own", UserGroupIds: []int64{10}}).Error)

	w := notificationOwnerRequest(t, rt.messageTemplatesDel, &models.User{Id: 1}, idsForm{Ids: []int64{1}}, "")

	require.Empty(t, notificationOwnerReply(t, w, nil))
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.MessageTemplate{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestMessageTemplatesGet_FiltersChannelAndPrivateOwnership(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.MessageTemplate{},
		&models.UserGroupMember{},
		&models.NotifyChannelConfig{},
		&models.User{},
	)
	seedNotificationMembership(t, rt)
	templates := []models.MessageTemplate{
		{Name: "public", Ident: "public", NotifyChannelIdent: "email", Private: 0},
		{Name: "own", Ident: "own", NotifyChannelIdent: "email", Private: 1, UserGroupIds: []int64{10}},
		{Name: "foreign", Ident: "foreign", NotifyChannelIdent: "email", Private: 1, UserGroupIds: []int64{20}},
		{Name: "other channel", Ident: "other", NotifyChannelIdent: "sms", Private: 0},
	}
	require.NoError(t, rt.Ctx.DB.Create(&templates).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.NotifyChannelConfig{ID: 5, Ident: "email"}).Error)

	w := notificationOwnerRequest(t, rt.messageTemplatesGet, &models.User{Id: 1}, nil, "?notify_channel_ids=5")

	var got []models.MessageTemplate
	require.Empty(t, notificationOwnerReply(t, w, &got))
	names := make([]string, 0, len(got))
	for _, tpl := range got {
		names = append(names, tpl.Name)
	}
	require.ElementsMatch(t, []string{"public", "own"}, names)
}

func TestNotifyRuleCustomParamsGet_FiltersTeamsChannelsAndKnownParameters(t *testing.T) {
	rt := notificationOwnerRouter(t,
		&models.NotifyRule{},
		&models.UserGroupMember{},
		&models.NotifyChannelConfig{},
	)
	seedNotificationMembership(t, rt)
	channel := models.NotifyChannelConfig{
		ID: 5, Ident: "custom",
		ParamConfig: &models.NotifyParamConfig{
			Custom: models.Params{
				Params: []models.ParamItem{
					{
						Key:   "room",
						CName: "Room",
					},
				},
			},
		},
	}
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)
	rules := []models.NotifyRule{
		{
			Name: "own", UserGroupIds: []int64{10},
			NotifyConfigs: []models.NotifyConfig{{ChannelID: 5, Params: map[string]interface{}{"room": "oncall"}}},
		},
		{
			Name: "other team", UserGroupIds: []int64{20},
			NotifyConfigs: []models.NotifyConfig{{ChannelID: 5, Params: map[string]interface{}{"room": "private"}}},
		},
		{
			Name: "other channel", UserGroupIds: []int64{10},
			NotifyConfigs: []models.NotifyConfig{{ChannelID: 6, Params: map[string]interface{}{"room": "different"}}},
		},
	}
	require.NoError(t, rt.Ctx.DB.Create(&rules).Error)

	w := notificationOwnerRequest(t, rt.notifyRuleCustomParamsGet, &models.User{Id: 1}, nil, "?notify_channel_id=5")

	var got [][]paramList
	require.Empty(t, notificationOwnerReply(t, w, &got))
	require.Equal(t, [][]paramList{{{Name: "room", CName: "Room", Value: "oncall"}}}, got)
}
