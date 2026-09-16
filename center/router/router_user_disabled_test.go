package router

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"sync"
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
		&models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// sqlite 的 :memory: 是每连接一个库，并发用例必须限定单连接，
	// 否则第二个 goroutine 会拿到一个空库
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

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

// 禁用最后一个可用管理员会让管理面彻底进不去，必须拦住；还有别的启用管理员时放行。
func TestUpdateDisabledKeepsLastEnabledAdmin(t *testing.T) {
	rt, _ := setupUserDisabledTest(t)

	alice := &models.User{Username: "alice", Roles: models.AdminRole, Contacts: []byte("{}")}
	if err := models.DB(rt.Ctx).Create(alice).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	if err := alice.UpdateDisabled(rt.Ctx, models.UserDisabled, "root"); err == nil {
		t.Fatalf("want error when disabling the last enabled admin, got nil")
	}

	// 再来一个启用的管理员，就可以禁用 alice 了
	bob := &models.User{Username: "bob", Roles: models.AdminRole, Contacts: []byte("{}")}
	if err := models.DB(rt.Ctx).Create(bob).Error; err != nil {
		t.Fatalf("seed second admin: %v", err)
	}
	if err := alice.UpdateDisabled(rt.Ctx, models.UserDisabled, "root"); err != nil {
		t.Fatalf("want no error when another enabled admin exists, got %v", err)
	}

	// alice 已被禁用，bob 成了最后一个可用管理员，禁不掉
	if err := bob.UpdateDisabled(rt.Ctx, models.UserDisabled, "root"); err == nil {
		t.Fatalf("want error when the only other admin is already disabled, got nil")
	}
}

// 「先冻结、后清理」是本功能的主线运维流程：已禁用的管理员不算可用管理员，
// 只要还有别的启用管理员，删除它就该放行。
func TestDelAllowsDisabledAdminButKeepsLastEnabledOne(t *testing.T) {
	rt, _ := setupUserDisabledTest(t)

	root := &models.User{Username: "root", Roles: models.AdminRole, Contacts: []byte("{}")}
	if err := models.DB(rt.Ctx).Create(root).Error; err != nil {
		t.Fatalf("seed root: %v", err)
	}
	alice := &models.User{Username: "alice", Roles: models.AdminRole, Contacts: []byte("{}"), Disabled: models.UserDisabled}
	if err := models.DB(rt.Ctx).Create(alice).Error; err != nil {
		t.Fatalf("seed disabled admin: %v", err)
	}

	target, err := models.UserGetById(rt.Ctx, alice.Id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if err := target.Del(rt.Ctx); err != nil {
		t.Fatalf("deleting a disabled admin should be allowed, got %v", err)
	}

	// root 是最后一个可用管理员，删不得
	target, err = models.UserGetById(rt.Ctx, root.Id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if err := target.Del(rt.Ctx); err == nil {
		t.Fatalf("want error when deleting the last enabled admin, got nil")
	}
}

// 列表分页总数要按状态过滤。
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

// 资料表单是先读后写的：读到禁用前的快照、期间管理员完成了禁用，
// 保存资料不能把 disabled 刷回 0，否则普通用户自己就能解除禁用。
func TestUpdateAllFieldsKeepsDisabled(t *testing.T) {
	rt, target := setupUserDisabledTest(t)

	stale, err := models.UserGetById(rt.Ctx, target.Id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	if err := target.UpdateDisabled(rt.Ctx, models.UserDisabled, "root"); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	stale.Nickname = "张三丰"
	if err := stale.UpdateAllFields(rt.Ctx); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	got, err := models.UserGetById(rt.Ctx, target.Id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !got.IsDisabled() {
		t.Fatalf("profile update must not resurrect a disabled account")
	}
	if got.Nickname != "张三丰" {
		t.Fatalf("profile update should still apply, got nickname=%q", got.Nickname)
	}
}

// 鉴权环节直接读库判断账号状态，不受用户缓存同秒不刷新的影响；
// 账号不存在也要如实报出来——删号不会清掉 Redis 里的会话，旧 token 还能撑一段时间，
// 期间若重建了同名账号，放行就等于把旧凭证接到新账号上。
func TestUserStatusById(t *testing.T) {
	rt, target := setupUserDisabledTest(t)

	exists, disabled, err := models.UserStatusById(rt.Ctx, target.Id)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if !exists || disabled {
		t.Fatalf("want exists&enabled, got exists=%v disabled=%v", exists, disabled)
	}

	if err := target.UpdateDisabled(rt.Ctx, models.UserDisabled, "root"); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	exists, disabled, err = models.UserStatusById(rt.Ctx, target.Id)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if !exists || !disabled {
		t.Fatalf("want exists&disabled, got exists=%v disabled=%v", exists, disabled)
	}

	exists, _, err = models.UserStatusById(rt.Ctx, 99999)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if exists {
		t.Fatalf("want not exists for a removed account")
	}
}

// 旧 token 的 userid 已经不存在时（账号被删、会话还没过期），鉴权必须拒绝，
// 否则重建的同名账号会被旧凭证接管。
func TestSetAuthUserRejectsMissingUser(t *testing.T) {
	rt, _ := setupUserDisabledTest(t)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	defer func() {
		if recover() == nil {
			t.Fatalf("want 401 for a userid that no longer exists")
		}
	}()

	rt.setAuthUser(c, 99999, "zhangsan")
}

// 「禁用 alice」和「删除 root」并发执行时，两条路径必须走同一把锁：
// 不管谁先谁后、谁失败，做完之后至少还得剩一个能登录的管理员。
func TestDisableAndDeleteAdminConcurrently(t *testing.T) {
	rt, _ := setupUserDisabledTest(t)

	root := &models.User{Username: "root", Roles: models.AdminRole, Contacts: []byte("{}")}
	if err := models.DB(rt.Ctx).Create(root).Error; err != nil {
		t.Fatalf("seed root: %v", err)
	}
	alice := &models.User{Username: "alice", Roles: models.AdminRole, Contacts: []byte("{}")}
	if err := models.DB(rt.Ctx).Create(alice).Error; err != nil {
		t.Fatalf("seed alice: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		alice.UpdateDisabled(rt.Ctx, models.UserDisabled, "root") //nolint:errcheck // 允许其中一方失败
	}()
	go func() {
		defer wg.Done()
		target := &models.User{Id: root.Id, Username: root.Username}
		target.Del(rt.Ctx) //nolint:errcheck // 允许其中一方失败
	}()
	wg.Wait()

	enabled := models.UserEnabled
	admins, err := models.UsersGet(rt.Ctx, "roles LIKE ? and disabled = ?", "%"+models.AdminRole+"%", enabled)
	if err != nil {
		t.Fatalf("list admins: %v", err)
	}
	if len(admins) < 1 {
		t.Fatalf("no enabled admin left: disable and delete must not both succeed")
	}
}
