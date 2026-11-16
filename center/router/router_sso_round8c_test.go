package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/feishu"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/stretchr/testify/require"
)

func TestR8cFeishuCallbackCreatesAndUpdatesIdentity(t *testing.T) {
	for _, mode := range []string{"new", "existing", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			rt, mr := sessionRouter(t)
			require.NoError(t, rt.Ctx.DB.AutoMigrate(&models.UserGroupMember{}))
			if mode != "new" {
				disabled := 0
				if mode == "disabled" {
					disabled = 1
				}
				require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "test@example.invalid", Nickname: "old", Disabled: disabled}).Error)
			}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.Contains(r.URL.Path, "access_token/internal"):
					fmt.Fprint(w, `{"code":0,"app_access_token":"app-test","tenant_access_token":"tenant-test","expire":0}`)
				case r.URL.Path == "/open-apis/authen/v1/access_token":
					var body map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["code"] != "code-test" {
						t.Errorf("wrong code: %v", body)
					}
					fmt.Fprint(w, `{"code":0,"data":{"access_token":"access-test","user_id":"user-test"}}`)
				case r.URL.Path == "/open-apis/contact/v3/users/user-test":
					fmt.Fprint(w, `{"code":0,"data":{"user":{"user_id":"user-test","name":"Updated Name","email":"test@example.invalid","mobile":"123"}}}`)
				default:
					t.Errorf("unexpected request %s", r.URL)
					http.Error(w, "unexpected", 404)
				}
			}))
			t.Cleanup(s.Close)
			old := lark.FeishuBaseUrl
			t.Cleanup(func() { lark.FeishuBaseUrl = old })
			rt.Sso.FeiShu = feishu.New(feishu.Config{Enable: true, AppID: t.Name(), AppSecret: "secret", FeiShuEndpoint: s.URL, CoverAttributes: true, DefaultRoles: []string{"Standard"}, DefaultUserGroups: []int64{10}})
			mr.Set("n9e_feishu_oauth_state", "/boards")
			w := r8cRequest(t, rt.loginCallbackFeiShu, nil, "/1?code=code-test&state=state")
			require.Equal(t, 200, w.Code, w.Body.String())
			if mode == "disabled" {
				require.Contains(t, w.Body.String(), models.ErrUserDisabled)
			} else {
				var reply CallbackOutput
				r8cData(t, w, &reply)
				require.Equal(t, "/boards", reply.Redirect)
				require.NotEmpty(t, reply.AccessToken)
			}
			var user models.User
			require.NoError(t, rt.Ctx.DB.Where("username = ?", "test@example.invalid").First(&user).Error)
			require.Equal(t, "Updated Name", user.Nickname)
			if mode == "new" {
				var members []models.UserGroupMember
				require.NoError(t, rt.Ctx.DB.Find(&members).Error)
				require.Len(t, members, 1)
				require.Equal(t, int64(10), members[0].GroupId)
			}
		})
	}
}
