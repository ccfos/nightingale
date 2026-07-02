package router

import (
	"context"
	"net/http"

	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/logx"

	"github.com/gin-gonic/gin"
)

// DatasourceMetaFilter 是 db-databases / db-tables / db-desc-table 三个
// 元数据接口的授权过滤 hook。默认 nil（保持任何登录用户可读全量元数据的
// 向后兼容行为），由外层增强实现按数据源授权配置做库表级过滤。
//
//   - FilterDatabases / FilterTables 语义为"筛出可见项"：无授权规则或用户是
//     管理员时返回原列表；否则返回过滤后的列表。
//   - CheckDescribeQuery 语义为"精确到 (database, table) 的前置校验"：query
//     是 datasource 特化的 interface{}（不同 datasource 结构不同），解析交给
//     增强实现；返回 false 时 handler 以 403 拒绝。
type DatasourceMetaFilter interface {
	FilterDatabases(ctx context.Context, user *models.User, cate string, datasourceID int64, databases []string) []string
	FilterTables(ctx context.Context, user *models.User, cate string, datasourceID int64, database string, tables []string) []string
	CheckDescribeQuery(ctx context.Context, user *models.User, cate string, datasourceID int64, query interface{}) bool
}

// MetaFilter 默认 nil，任何登录用户可读全量元数据。增强实现由外层注入。
var MetaFilter DatasourceMetaFilter

// resolveMetaFilterUser 返回可用于 MetaFilter 的 (user, filter, ok)。
// 未注入 filter、无 user 上下文（如 AnonymousAccess 分支）、user 类型不匹配时
// 返回 ok=false，handler 侧应视为"不过滤（放行）"。
func resolveMetaFilterUser(c *gin.Context) (*models.User, DatasourceMetaFilter, bool) {
	filter := MetaFilter
	if filter == nil {
		return nil, nil, false
	}
	v, exists := c.Get("user")
	if !exists {
		return nil, nil, false
	}
	user, ok := v.(*models.User)
	if !ok || user == nil {
		return nil, nil, false
	}
	return user, filter, true
}

func (rt *Router) ShowDatabases(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logx.Warningf(c.Request.Context(), "cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	var databases []string
	var err error
	type DatabaseShower interface {
		ShowDatabases(context.Context) ([]string, error)
	}
	switch plug.(type) {
	case DatabaseShower:
		databases, err = plug.(DatabaseShower).ShowDatabases(c.Request.Context())
		ginx.Dangerous(err)
	default:
		ginx.Bomb(200, "datasource not exists")
	}

	if len(databases) == 0 {
		databases = make([]string, 0)
	}

	if user, filter, ok := resolveMetaFilterUser(c); ok {
		databases = filter.FilterDatabases(c.Request.Context(), user, f.Cate, f.DatasourceId, databases)
		if databases == nil {
			databases = make([]string, 0)
		}
	}

	ginx.NewRender(c).Data(databases, nil)
}

func (rt *Router) ShowTables(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logx.Warningf(c.Request.Context(), "cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	// 只接受一个入参
	tables := make([]string, 0)
	var err error
	type TableShower interface {
		ShowTables(ctx context.Context, database string) ([]string, error)
	}
	switch plug.(type) {
	case TableShower:
		if len(f.Queries) > 0 {
			database, ok := f.Queries[0].(string)
			if ok {
				tables, err = plug.(TableShower).ShowTables(c.Request.Context(), database)
				if user, filter, ok := resolveMetaFilterUser(c); ok {
					tables = filter.FilterTables(c.Request.Context(), user, f.Cate, f.DatasourceId, database, tables)
					if tables == nil {
						tables = make([]string, 0)
					}
				}
			}
		}
	default:
		ginx.Bomb(200, "datasource not exists")
	}
	ginx.NewRender(c).Data(tables, err)
}

func (rt *Router) DescribeTable(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logx.Warningf(c.Request.Context(), "cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}
	// 只接受一个入参
	columns := make([]*types.ColumnProperty, 0)
	var err error
	type TableDescriber interface {
		DescribeTable(context.Context, interface{}) ([]*types.ColumnProperty, error)
	}
	switch plug.(type) {
	case TableDescriber:
		client := plug.(TableDescriber)
		if len(f.Queries) > 0 {
			if user, filter, ok := resolveMetaFilterUser(c); ok {
				if !filter.CheckDescribeQuery(c.Request.Context(), user, f.Cate, f.DatasourceId, f.Queries[0]) {
					ginx.Bomb(http.StatusForbidden, "no permission")
				}
			}
			columns, err = client.DescribeTable(c.Request.Context(), f.Queries[0])
		}
	default:
		ginx.Bomb(200, "datasource not exists")
	}

	ginx.NewRender(c).Data(columns, err)
}
