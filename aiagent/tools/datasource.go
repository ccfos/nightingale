package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
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
	register(defs.ListDatasources, listDatasourcesBuiltin)
	register(defs.GetDatasourceDetail, getDatasourceDetail)
}

func listDatasourcesBuiltin(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}

	pluginType := getArgString(args, "plugin_type")
	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	dsList, err := models.GetDatasourcesGetsBy(deps.DBCtx, pluginType, "", query, "")
	if err != nil {
		return "", fmt.Errorf("failed to query datasources: %v", err)
	}

	// Apply the same DatasourceFilter hook used by the web UI
	filtered := deps.FilterDatasources(dsList, user)

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

func getDatasourceDetail(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	ds, err := models.DatasourceGet(deps.DBCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get datasource: %v", err)
	}
	if ds == nil {
		return fmt.Sprintf(`{"error":"datasource not found: id=%d"}`, id), nil
	}

	// Verify visibility via the same DatasourceFilter hook used by the web UI
	filtered := deps.FilterDatasources([]*models.Datasource{ds}, user)
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
