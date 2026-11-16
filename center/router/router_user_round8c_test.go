package router

import (
	"errors"
	"fmt"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestR8cUserInstallDate(t *testing.T) {
	rt := r8cRouter(t, &models.User{})
	var timestamp int64
	r8cData(t, r8cRequest(t, rt.installDateGet, nil, "/1"), &timestamp)
	require.Zero(t, timestamp)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "root", CreateAt: 12345}).Error)
	r8cData(t, r8cRequest(t, rt.installDateGet, nil, "/1"), &timestamp)
	require.Equal(t, int64(12345), timestamp)
	require.NoError(t, rt.Ctx.DB.Migrator().DropTable(&models.User{}))
	r8cData(t, r8cRequest(t, rt.installDateGet, nil, "/1"), &timestamp)
	require.Zero(t, timestamp)
}
func TestR8cPhoneEncryptionStorageFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		handler   func(*Router) gin.HandlerFunc
		message   string
		userTable bool
	}{
		{"encrypt users missing", func(rt *Router) gin.HandlerFunc { return rt.usersPhoneEncrypt }, "get users failed", false},
		{"encrypt config missing", func(rt *Router) gin.HandlerFunc { return rt.usersPhoneEncrypt }, "get RSA keys failed", true},
		{"decrypt config missing", func(rt *Router) gin.HandlerFunc { return rt.usersPhoneDecrypt }, "disable phone encryption failed", false},
		{"refresh config missing", func(rt *Router) gin.HandlerFunc { return rt.usersPhoneDecryptRefresh }, "refresh phone encryption cache failed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := r8cRouter(t)
			if tc.userTable {
				require.NoError(t, rt.Ctx.DB.AutoMigrate(&models.User{}))
			}
			w := r8cRequest(t, tc.handler(rt), nil, "/1")
			require.Equal(t, 200, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
		})
	}
}
func TestR8cUserRSAMutationsRejectInvalidCiphertext(t *testing.T) {
	for _, h := range []string{"add", "profile", "password"} {
		t.Run(h, func(t *testing.T) {
			rt := r8cRouter(t, &models.User{}, &models.Configs{})
			rt.HTTP.RSA.OpenRSA = true
			require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Password: "unchanged", Roles: "Standard"}).Error)
			var handler gin.HandlerFunc
			body := map[string]interface{}{"username": "alice", "password": "invalid", "roles": []string{"Admin"}}
			switch h {
			case "add":
				handler = rt.userAddPost
			case "profile":
				handler = rt.userProfilePutByService
			case "password":
				handler = rt.userPasswordPut
			}
			w := r8cRequest(t, handler, body, "/1")
			require.Equal(t, 200, w.Code)
			require.Contains(t, w.Body.String(), "err")
			var u models.User
			require.NoError(t, rt.Ctx.DB.First(&u, 1).Error)
			require.Equal(t, "unchanged", u.Password)
			require.Equal(t, "Standard", u.Roles)
		})
	}
}

func TestR8cPhoneEncryptionRollsBackConfigurationOnRefreshFailure(t *testing.T) {
	for _, encrypt := range []bool{true, false} {
		t.Run(fmt.Sprint(encrypt), func(t *testing.T) {
			rt := r8cRouter(t, &models.Configs{}, &models.User{})
			initial := "true"
			failAt := 2
			if encrypt {
				initial = "false"
				failAt = 5
			}
			require.NoError(t, rt.Ctx.DB.Create(&models.Configs{Ckey: models.PHONE_ENCRYPTION_ENABLED, Cval: initial}).Error)
			queries := 0
			require.NoError(t, rt.Ctx.DB.Callback().Query().Before("gorm:query").Register("r8c_refresh_failure", func(db *gorm.DB) {
				if db.Statement.Table == "configs" {
					queries++
					if queries == failAt {
						db.AddError(errors.New("config read unavailable"))
					}
				}
			}))
			handler := rt.usersPhoneDecrypt
			if encrypt {
				handler = rt.usersPhoneEncrypt
			}
			w := r8cRequest(t, handler, nil, "/1")
			require.Equal(t, 200, w.Code)
			require.Contains(t, w.Body.String(), "refresh cache failed")
			var saved models.Configs
			require.NoError(t, rt.Ctx.DB.Where("ckey = ?", models.PHONE_ENCRYPTION_ENABLED).First(&saved).Error)
			require.Equal(t, initial, saved.Cval)
		})
	}
}
func TestR8cPhoneEncryptionEnableFailure(t *testing.T) {
	rt := r8cRouter(t, &models.Configs{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Callback().Create().Before("gorm:create").Register("r8c_save_fail", func(db *gorm.DB) { db.AddError(errors.New("write unavailable")) }))
	w := r8cRequest(t, rt.usersPhoneEncrypt, nil, "/1")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "enable phone encryption failed")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Configs{}).Count(&count).Error)
	require.Zero(t, count)
}
