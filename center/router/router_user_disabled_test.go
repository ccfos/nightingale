package router

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupUserDisabledTest 准备一个基于内存 sqlite 的 Router，并预置一个待禁用的普通用户。
func setupUserDisabledTest(t *testing.T) (*Router, *models.User) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.UserGroup{}, &models.UserGroupMember{},
		&models.BusiGroup{}, &models.BusiGroupMember{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rt := &Router{Ctx: &ctx.Context{DB: db}}

	target := &models.User{
		Username: "zhangsan",
		Nickname: "张三",
		Roles:    "Standard",
		Contacts: []byte("{}"),
	}
	if err := db.Create(target).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	return rt, target
}

// callUserDisabledPut 直接调用 userDisabledPut 处理函数，模拟一次禁用/启用请求。
func callUserDisabledPut(t *testing.T, rt *Router, operator *models.User, targetId int64, disabled int) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body, err := json.Marshal(userDisabledForm{Disabled: disabled})
	if err != nil {
		t.Fatalf("marshal form: %v", err)
	}
	c.Request = httptest.NewRequest("PUT", "/api/n9e/user/1/disabled", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(targetId, 10)}}
	c.Set("user", operator)

	rt.userDisabledPut(c)
}

// 禁用只翻转状态位，角色、昵称等配置原样保留，启用后用户不需要重新配置权限。
func TestUserDisabledPutKeepsOtherFields(t *testing.T) {
	rt, target := setupUserDisabledTest(t)
	operator := &models.User{Id: 999, Username: "root"}

	callUserDisabledPut(t, rt, operator, target.Id, models.UserDisabled)

	got, err := models.UserGetById(rt.Ctx, target.Id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !got.IsDisabled() {
		t.Fatalf("want user disabled, got disabled=%d", got.Disabled)
	}
	if got.Roles != "Standard" || got.Nickname != "张三" {
		t.Fatalf("disable must not touch other fields, got roles=%q nickname=%q", got.Roles, got.Nickname)
	}
	if got.UpdateBy != "root" {
		t.Fatalf("want update_by=root, got %q", got.UpdateBy)
	}

	callUserDisabledPut(t, rt, operator, target.Id, models.UserEnabled)

	got, err = models.UserGetById(rt.Ctx, target.Id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.IsDisabled() {
		t.Fatalf("want user enabled again, got disabled=%d", got.Disabled)
	}
	if got.Roles != "Standard" {
		t.Fatalf("want roles kept after re-enable, got %q", got.Roles)
	}
}

func TestCheckUserCanBeDisabled(t *testing.T) {
	me := &models.User{Id: 1, Username: "admin"}

	cases := []struct {
		name    string
		target  *models.User
		wantErr bool
	}{
		{"self", &models.User{Id: 1, Username: "admin"}, true},
		{"root", &models.User{Id: 2, Username: "root"}, true},
		{"normal user", &models.User{Id: 3, Username: "zhangsan"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkUserCanBeDisabled(me, tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want no error, got %v", err)
			}
		})
	}
}

// 列表按状态筛选：不传 disabled 返回全部，传 0/1 分别只返回正常/已禁用的账号。
func TestUserTotalFilterByDisabled(t *testing.T) {
	rt, _ := setupUserDisabledTest(t)
	if err := models.DB(rt.Ctx).Create(&models.User{Username: "lisi", Contacts: []byte("{}"), Disabled: models.UserDisabled}).Error; err != nil {
		t.Fatalf("seed disabled user: %v", err)
	}

	all, err := models.UserTotal(rt.Ctx, "", 0, 0, nil)
	if err != nil {
		t.Fatalf("count all: %v", err)
	}
	if all != 2 {
		t.Fatalf("want 2 users, got %d", all)
	}

	enabled := models.UserEnabled
	num, err := models.UserTotal(rt.Ctx, "", 0, 0, &enabled)
	if err != nil {
		t.Fatalf("count enabled: %v", err)
	}
	if num != 1 {
		t.Fatalf("want 1 enabled user, got %d", num)
	}

	disabled := models.UserDisabled
	num, err = models.UserTotal(rt.Ctx, "", 0, 0, &disabled)
	if err != nil {
		t.Fatalf("count disabled: %v", err)
	}
	if num != 1 {
		t.Fatalf("want 1 disabled user, got %d", num)
	}
}

// 列表数据同样要按状态过滤：UserTotal 与 UserGets 各自拼条件，只测其中一个，
// 另一个写错就会出现「总数说 1 条、表格里却是 2 行」。
func TestUserGetsFilterByDisabled(t *testing.T) {
	rt, _ := setupUserDisabledTest(t)
	if err := models.DB(rt.Ctx).Create(&models.User{Username: "lisi", Contacts: []byte("{}"), Disabled: models.UserDisabled}).Error; err != nil {
		t.Fatalf("seed disabled user: %v", err)
	}

	all, err := models.UserGets(rt.Ctx, "", 10, 0, 0, 0, "username", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 users, got %d", len(all))
	}

	enabled := models.UserEnabled
	list, err := models.UserGets(rt.Ctx, "", 10, 0, 0, 0, "username", false, nil, nil, nil, &enabled)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(list) != 1 || list[0].Username != "zhangsan" {
		t.Fatalf("want only zhangsan enabled, got %+v", list)
	}

	disabled := models.UserDisabled
	list, err = models.UserGets(rt.Ctx, "", 10, 0, 0, 0, "username", false, nil, nil, nil, &disabled)
	if err != nil {
		t.Fatalf("list disabled: %v", err)
	}
	if len(list) != 1 || list[0].Username != "lisi" {
		t.Fatalf("want only lisi disabled, got %+v", list)
	}
}

// 零值 Id 的 User 不能把全表刷成禁用。
func TestUpdateDisabledRejectsZeroId(t *testing.T) {
	rt, target := setupUserDisabledTest(t)

	if err := (&models.User{}).UpdateDisabled(rt.Ctx, models.UserDisabled, "root"); err == nil {
		t.Fatalf("want error when user id is zero, got nil")
	}

	got, err := models.UserGetById(rt.Ctx, target.Id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.IsDisabled() {
		t.Fatalf("existing user must stay enabled, got disabled=%d", got.Disabled)
	}
}
