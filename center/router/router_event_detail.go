package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// eventDetailPage renders an HTML log viewer page (for pages group).
func (rt *Router) eventDetailPage(c *gin.Context) {
	hash := ginx.UrlParamStr(c, "hash")
	if !loggrep.IsValidHash(hash) {
		c.String(http.StatusBadRequest, "invalid hash format")
		return
	}

	logs, instance, err := rt.getEventLogs(hash)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	err = loggrep.RenderHTML(c.Writer, loggrep.PageData{
		Hash:     hash,
		Instance: instance,
		Logs:     logs,
		Total:    len(logs),
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "render error: %v", err)
	}
}

// eventDetailJSON returns JSON (for service group).
func (rt *Router) eventDetailJSON(c *gin.Context) {
	hash := ginx.UrlParamStr(c, "hash")
	if !loggrep.IsValidHash(hash) {
		ginx.Bomb(200, "invalid hash format")
	}

	logs, instance, err := rt.getEventLogs(hash)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(loggrep.EventDetailResp{
		Logs:     logs,
		Instance: instance,
	}, nil)
}

// getNodeForDatasource returns the alert engine instance responsible for the given
// datasource and primary key. It first checks the local hashring, and falls back
// to querying the database for active instances if the hashring is empty
// (e.g. when the datasource belongs to another engine cluster).
func (rt *Router) getNodeForDatasource(datasourceId int64, pk string) (string, error) {
	dsIdStr := strconv.FormatInt(datasourceId, 10)
	node, err := naming.DatasourceHashRing.GetNode(dsIdStr, pk)
	if err == nil {
		return node, nil
	}

	// Hashring is empty for this datasource (likely belongs to another engine cluster).
	// Query the DB for active instances.
	servers, dbErr := models.AlertingEngineGetsInstances(rt.Ctx,
		"datasource_id = ? and clock > ?",
		datasourceId, time.Now().Unix()-30)
	if dbErr != nil {
		return "", dbErr
	}
	if len(servers) == 0 {
		return "", fmt.Errorf("no active instances for datasource %d", datasourceId)
	}

	ring := naming.NewConsistentHashRing(int32(naming.NodeReplicas), servers)
	return ring.Get(pk)
}

// getEventLogs resolves the target instance and retrieves logs.
func (rt *Router) getEventLogs(hash string) ([]string, string, error) {
	event, err := models.AlertHisEventGetByHash(rt.Ctx, hash)
	if err != nil {
		return nil, "", err
	}
	if event == nil {
		return nil, "", fmt.Errorf("no such alert event")
	}

	ruleId := strconv.FormatInt(event.RuleId, 10)

	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)

	node, err := rt.getNodeForDatasource(event.DatasourceId, ruleId)
	if err != nil || node == instance {
		// hashring not ready or target is self, handle locally
		logs, err := loggrep.GrepLogDir(rt.LogDir, hash)
		return logs, instance, err
	}

	// forward to the target alert instance
	return rt.forwardEventDetail(node, hash)
}

func (rt *Router) forwardEventDetail(node, hash string) ([]string, string, error) {
	url := fmt.Sprintf("http://%s/v1/n9e/event-detail/%s", node, hash)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, node, err
	}

	for user, pass := range rt.HTTP.APIForService.BasicAuth {
		req.SetBasicAuth(user, pass)
		break
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, node, fmt.Errorf("forward to %s failed: %v", node, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, node, err
	}

	var result struct {
		Dat loggrep.EventDetailResp `json:"dat"`
		Err string                  `json:"err"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, node, err
	}
	if result.Err != "" {
		return nil, node, fmt.Errorf("%s", result.Err)
	}

	return result.Dat.Logs, result.Dat.Instance, nil
}
