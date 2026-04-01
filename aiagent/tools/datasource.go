package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type datasourceResult struct {
	Id             int64  `json:"id"`
	Name           string `json:"name"`
	PluginType     string `json:"plugin_type"`
	PluginTypeName string `json:"plugin_type_name,omitempty"`
	Category       string `json:"category,omitempty"`
	Status         string `json:"status,omitempty"`
}

type datasourceDetailResult struct {
	Id             int64  `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	PluginType     string `json:"plugin_type"`
	PluginTypeName string `json:"plugin_type_name,omitempty"`
	Category       string `json:"category,omitempty"`
	ClusterName    string `json:"cluster_name,omitempty"`
	Status         string `json:"status,omitempty"`
	IsDefault      bool   `json:"is_default"`
	CreatedBy      string `json:"created_by,omitempty"`
	UpdatedBy      string `json:"updated_by,omitempty"`
}

func init() {
	register("list_datasources", aiagent.AgentTool{
		Name:        "list_datasources",
		Description: "查询数据源列表，支持按类型过滤",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "plugin_type", Type: "string", Description: "数据源类型过滤，如 prometheus、mysql、elasticsearch", Required: false},
			{Name: "query", Type: "string", Description: "搜索关键词，匹配数据源名称", Required: false},
			{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
		},
	}, listDatasourcesBuiltin)

	register("get_datasource_detail", aiagent.AgentTool{
		Name:        "get_datasource_detail",
		Description: "获取单个数据源的详细信息",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "id", Type: "integer", Description: "数据源ID", Required: true},
		},
	}, getDatasourceDetail)
}

func listDatasourcesBuiltin(_ context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(params)
	if err != nil {
		return "", err
	}

	pluginType := getArgString(args, "plugin_type")
	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	dbCtx := aiagent.GetDBCtx()
	dsList, err := models.GetDatasourcesGetsBy(dbCtx, pluginType, "", query, "")
	if err != nil {
		return "", fmt.Errorf("failed to query datasources: %v", err)
	}

	// Apply the same DatasourceFilter hook used by the web UI
	filtered := aiagent.FilterDatasources(dsList, user)

	results := make([]datasourceResult, 0)
	for _, ds := range filtered {
		results = append(results, datasourceResult{
			Id:             ds.Id,
			Name:           ds.Name,
			PluginType:     ds.PluginType,
			PluginTypeName: ds.PluginTypeName,
			Category:       ds.Category,
			Status:         ds.Status,
		})
		if len(results) >= limit {
			break
		}
	}

	logger.Debugf("list_datasources: plugin_type=%s, found %d datasources", pluginType, len(results))
	return marshalList(len(results), results), nil
}

func getDatasourceDetail(_ context.Context, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(params)
	if err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	dbCtx := aiagent.GetDBCtx()
	ds, err := models.DatasourceGet(dbCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get datasource: %v", err)
	}
	if ds == nil {
		return fmt.Sprintf(`{"error":"datasource not found: id=%d"}`, id), nil
	}

	// Verify visibility via the same DatasourceFilter hook used by the web UI
	filtered := aiagent.FilterDatasources([]*models.Datasource{ds}, user)
	if len(filtered) == 0 {
		return "", fmt.Errorf("forbidden: no access to this datasource")
	}

	result := datasourceDetailResult{
		Id:             ds.Id,
		Name:           ds.Name,
		Description:    ds.Description,
		PluginType:     ds.PluginType,
		PluginTypeName: ds.PluginTypeName,
		Category:       ds.Category,
		ClusterName:    ds.ClusterName,
		Status:         ds.Status,
		IsDefault:      ds.IsDefault,
		CreatedBy:      ds.CreatedBy,
		UpdatedBy:      ds.UpdatedBy,
	}

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
