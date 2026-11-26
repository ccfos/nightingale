package router

import (
	"strings"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/grafana"

	"github.com/gin-gonic/gin"
)

// grafanaPreviewItem 是「拉取预览」返回给前端的单条：内嵌映射标注，
// 附带是否与已有数据源重名，以及已构建好、可原样回传给 import 的 Datasource。
type grafanaPreviewItem struct {
	grafana.PreviewMeta
	Duplicate  bool               `json:"duplicate"`
	Datasource *models.Datasource `json:"datasource"`
}

// datasourceGrafanaFetch 连接 Grafana 拉取数据源清单，逐条映射为 n9e 数据源并标注
// supported / duplicate / need_auth，返回预览列表。此接口不写库。
// supported 依据当前启用的目录（rt.Center.Plugins），而非全量静态表，
// 避免把当前部署未启用的类型标成可导入。
func (rt *Router) datasourceGrafanaFetch(c *gin.Context) {
	// 与 upsert/status/delete 一致：ClustersFromAPIs 模式下本地数据源 API 被禁用，直接返回空。
	if rt.DatasourceCache.DatasourceCheckHook(c) {
		Render(c, []int{}, nil)
		return
	}

	var conn grafana.Conn
	ginx.BindJSON(c, &conn)

	gdss, err := grafana.FetchDatasources(conn)
	if err != nil {
		Render(c, nil, err.Error())
		return
	}

	// 先映射并收集受支持项的名称，一次性批量查重（避免逐条 N+1 查询）。
	items := make([]grafanaPreviewItem, 0, len(gdss))
	names := make([]string, 0, len(gdss))
	for _, gds := range gdss {
		ds, meta := grafana.MapToDatasource(gds, rt.Center.Plugins)
		items = append(items, grafanaPreviewItem{PreviewMeta: meta, Datasource: ds})
		if ds != nil {
			names = append(names, ds.Name)
		}
	}

	existing, err := rt.existingDatasourceNames(names)
	if err != nil {
		Render(c, nil, err.Error())
		return
	}
	for i := range items {
		if items[i].Datasource != nil {
			items[i].Duplicate = existing[strings.ToLower(items[i].Datasource.Name)]
		}
	}

	Render(c, gin.H{"items": items}, nil)
}

// existingDatasourceNames 用一次 WHERE name IN (?) 查询出已存在的数据源名集合，替代逐条查重。
func (rt *Router) existingDatasourceNames(names []string) (map[string]bool, error) {
	set := make(map[string]bool)
	if len(names) == 0 {
		return set, nil
	}
	var existing []string
	err := models.DB(rt.Ctx).Model(&models.Datasource{}).Where("name in ?", names).Pluck("name", &existing).Error
	if err != nil {
		return nil, err
	}
	// 用小写归一 key：datasource.name 唯一索引常是大小写不敏感 collation，
	// 库里的 foo 与待导入的 Foo 应判为同名。
	for _, n := range existing {
		set[strings.ToLower(n)] = true
	}
	return set, nil
}

// enabledPluginTypes 返回当前启用的 plugin_type 集合，用于导入时服务端复核。
func enabledPluginTypes(plugins []cconf.Plugin) map[string]bool {
	set := make(map[string]bool, len(plugins))
	for _, p := range plugins {
		set[p.Type] = true
	}
	return set
}

// clusterNameFromSettings 从 settings 里取出「关联告警引擎集群」（键形如 {type}.cluster_name）。
// 与 datasourceUpsert 的取法一致；区别是这里的 settings 直接来自客户端提交，故用带 ok 的类型断言，
// 非字符串值只当作未设置，而不是像 upsert 里 v.(string) 那样直接 panic。
func clusterNameFromSettings(settings map[string]interface{}) string {
	for k, v := range settings {
		if !strings.Contains(k, "cluster_name") {
			continue
		}
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// grafanaImportResult 是「批量导入」每条数据源的处理结果。
// status: imported（已启用）/ pending_auth（缺密钥，存为 disabled 待补填）/ skipped（重名）/ failed。
type grafanaImportResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// datasourceGrafanaImport 接收前端勾选回传的 Datasource 列表，逐条落库。
// 仿 alertRuleAddByImport：重名跳过，其余直接 Add（刻意不走 upsert 的连通性校验，
// 因为 need_auth 的源缺密钥必然连不通），按 Status 汇总每条结果。
func (rt *Router) datasourceGrafanaImport(c *gin.Context) {
	// ClustersFromAPIs 模式下本地数据源为只读，禁止落库（与 upsert/status/delete 一致）。
	if rt.DatasourceCache.DatasourceCheckHook(c) {
		Render(c, []int{}, nil)
		return
	}

	var items []models.Datasource
	ginx.BindJSON(c, &items)
	if len(items) == 0 {
		Render(c, nil, "input json is empty")
		return
	}

	username := Username(c)
	enabled := enabledPluginTypes(rt.Center.Plugins)

	names := make([]string, 0, len(items))
	for i := range items {
		names = append(names, items[i].Name)
	}
	existing, err := rt.existingDatasourceNames(names)
	if err != nil {
		Render(c, nil, err.Error())
		return
	}

	results := make([]grafanaImportResult, 0, len(items))
	for i := range items {
		ds := &items[i]
		res := grafanaImportResult{Name: ds.Name}

		// 服务端复核：类型必须在当前启用的目录内，防止客户端绕过预览直接提交未启用类型。
		if !enabled[ds.PluginType] {
			res.Status, res.Reason = "failed", "unsupported type"
			results = append(results, res)
			continue
		}
		key := strings.ToLower(ds.Name)
		if existing[key] {
			res.Status, res.Reason = "skipped", "name already exists"
			results = append(results, res)
			continue
		}

		ds.CreatedBy = username
		ds.UpdatedBy = username
		if ds.Status == "" {
			ds.Status = "enabled"
		}
		// 与 datasourceUpsert 保持一致：前端表单把「关联告警引擎集群」存在 settings 的
		// {type}.cluster_name 里，cluster_name 列由它派生（dscache 的 edge 过滤读的是列）。
		// 导入是另一条落库路径，不补这段抄写的话，前端设置的关联引擎不会写进列、edge 过滤失效。
		ds.ClusterName = clusterNameFromSettings(ds.SettingsJson)

		if err := ds.Add(rt.Ctx); err != nil {
			// 唯一索引在大小写不敏感 collation / 并发下可能拦下预检没发现的重名，兜底判 skipped。
			if cnt, _ := models.GetDatasourcesCountByName(rt.Ctx, ds.Name); cnt > 0 {
				res.Status, res.Reason = "skipped", "name already exists"
			} else {
				res.Status, res.Reason = "failed", err.Error()
			}
		} else {
			existing[key] = true // 拦住同批次内的重复名
			if ds.Status == "disabled" {
				res.Status = "pending_auth"
			} else {
				res.Status = "imported"
			}
		}
		results = append(results, res)
	}

	Render(c, gin.H{"items": results}, nil)
}
