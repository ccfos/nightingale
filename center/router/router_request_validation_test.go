package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWriteHandlers_RejectMalformedJSONBeforeAccessingDatabase(t *testing.T) {
	rt := &Router{}
	cases := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "alert rule add", handler: rt.alertRuleAddByFE},
		{name: "alert rule import", handler: rt.alertRuleAddByImport},
		{name: "alert rule update", handler: rt.alertRulePutByFE},
		{name: "alert rule fields", handler: rt.alertRulePutFields},
		{name: "alert mute add", handler: rt.alertMuteAdd},
		{name: "alert mute update", handler: rt.alertMutePutByFE},
		{name: "alert subscribe add", handler: rt.alertSubscribeAdd},
		{name: "alert subscribe update", handler: rt.alertSubscribePut},
		{name: "notify rule add", handler: rt.notifyRulesAdd},
		{name: "notify channel add", handler: rt.notifyChannelsAdd},
		{name: "message template add", handler: rt.messageTemplatesAdd},
		{name: "user add", handler: rt.userAddPost},
		{name: "user profile", handler: rt.userProfilePut},
		{name: "user group add", handler: rt.userGroupAdd},
		{name: "business group add", handler: rt.busiGroupAdd},
		{name: "recording rule add", handler: rt.recordingRuleAddByFE},
		{name: "pipeline add", handler: rt.addEventPipeline},
		{name: "board add", handler: rt.boardAdd},
		{name: "target tags", handler: rt.targetBindTagsByFE},
		{name: "target group binding", handler: rt.targetBindBgids},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(aop.Recovery())
			engine.Use(func(c *gin.Context) {
				c.Set("username", "editor")
				c.Set("user", &models.User{Username: "editor"})
			})
			engine.POST("/objects", tc.handler)
			req := httptest.NewRequest(http.MethodPost, "/objects", strings.NewReader(`{"unfinished":`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			engine.ServeHTTP(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "json body invalid")
			require.Contains(t, w.Body.String(), "unexpected EOF")
		})
	}
}

func TestBatchDeleteHandlers_RejectMissingIDsBeforeAccessingDatabase(t *testing.T) {
	rt := &Router{}
	cases := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "mute", handler: rt.alertMuteDel},
		{name: "subscribe", handler: rt.alertSubscribeDel},
		{name: "notification rules", handler: rt.notifyRulesDel},
		{name: "notification channels", handler: rt.notifyChannelsDel},
		{name: "message templates", handler: rt.messageTemplatesDel},
		{name: "recording rules", handler: rt.recordingRuleDel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(aop.Recovery())
			engine.POST("/objects", tc.handler)
			req := httptest.NewRequest(http.MethodPost, "/objects", strings.NewReader(`{"ids":[]}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			engine.ServeHTTP(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "ids empty")
		})
	}
}
