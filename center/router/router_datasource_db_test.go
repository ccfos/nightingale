package router

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
)

// stubMetaFilter 是测试用的 DatasourceMetaFilter mock，字段全 nil 时行为等同
// 未注入，赋值成对应的 hook 后可用于断言参数与返回值。
type stubMetaFilter struct {
	filterDatabases    func(context.Context, *models.User, string, int64, []string) []string
	filterTables       func(context.Context, *models.User, string, int64, string, []string) []string
	checkDescribeQuery func(context.Context, *models.User, string, int64, interface{}) bool
}

func (s *stubMetaFilter) FilterDatabases(ctx context.Context, user *models.User, cate string, dsID int64, dbs []string) []string {
	return s.filterDatabases(ctx, user, cate, dsID, dbs)
}
func (s *stubMetaFilter) FilterTables(ctx context.Context, user *models.User, cate string, dsID int64, db string, tables []string) []string {
	return s.filterTables(ctx, user, cate, dsID, db, tables)
}
func (s *stubMetaFilter) CheckDescribeQuery(ctx context.Context, user *models.User, cate string, dsID int64, query interface{}) bool {
	return s.checkDescribeQuery(ctx, user, cate, dsID, query)
}

// withMetaFilter 临时替换包级 MetaFilter，测试结束恢复。
func withMetaFilter(t *testing.T, f DatasourceMetaFilter) {
	t.Helper()
	prev := MetaFilter
	MetaFilter = f
	t.Cleanup(func() { MetaFilter = prev })
}

func TestResolveMetaFilterUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newCtx := func(setUser bool, val any) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if setUser {
			c.Set("user", val)
		}
		return c
	}

	t.Run("filter nil -> ok=false", func(t *testing.T) {
		withMetaFilter(t, nil)
		c := newCtx(true, &models.User{Id: 1})
		user, filter, ok := resolveMetaFilterUser(c)
		if ok || user != nil || filter != nil {
			t.Fatalf("want (nil, nil, false), got (%v, %v, %v)", user, filter, ok)
		}
	})

	t.Run("filter set but no user in context -> ok=false", func(t *testing.T) {
		withMetaFilter(t, &stubMetaFilter{})
		c := newCtx(false, nil)
		user, filter, ok := resolveMetaFilterUser(c)
		if ok || user != nil || filter != nil {
			t.Fatalf("want (nil, nil, false), got (%v, %v, %v)", user, filter, ok)
		}
	})

	t.Run("filter set but user is wrong type -> ok=false", func(t *testing.T) {
		withMetaFilter(t, &stubMetaFilter{})
		c := newCtx(true, "not-a-user-pointer")
		user, filter, ok := resolveMetaFilterUser(c)
		if ok || user != nil || filter != nil {
			t.Fatalf("want (nil, nil, false), got (%v, %v, %v)", user, filter, ok)
		}
	})

	t.Run("filter set but user is nil pointer -> ok=false", func(t *testing.T) {
		withMetaFilter(t, &stubMetaFilter{})
		var nilUser *models.User
		c := newCtx(true, nilUser)
		user, filter, ok := resolveMetaFilterUser(c)
		if ok || user != nil || filter != nil {
			t.Fatalf("want (nil, nil, false), got (%v, %v, %v)", user, filter, ok)
		}
	})

	t.Run("filter set and user valid -> ok=true and passes through", func(t *testing.T) {
		want := &models.User{Id: 42, Username: "alice"}
		stub := &stubMetaFilter{}
		withMetaFilter(t, stub)
		c := newCtx(true, want)
		user, filter, ok := resolveMetaFilterUser(c)
		if !ok {
			t.Fatalf("want ok=true, got false")
		}
		if user != want {
			t.Fatalf("user pointer mismatch: want %p got %p", want, user)
		}
		if filter != stub {
			t.Fatalf("filter identity mismatch: want %p got %p", stub, filter)
		}
	})
}
