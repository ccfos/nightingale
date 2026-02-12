package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
)

// traceLogsPage renders an HTML log viewer page for trace logs.
func (rt *Router) traceLogsPage(c *gin.Context) {
	traceId := ginx.UrlParamStr(c, "traceid")
	if !loggrep.IsValidTraceID(traceId) {
		c.String(http.StatusBadRequest, "invalid trace id format")
		return
	}

	logs, instance, err := rt.getTraceLogs(traceId)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	err = loggrep.RenderTraceLogsHTML(c.Writer, loggrep.TraceLogsPageData{
		TraceID:  traceId,
		Instance: instance,
		Logs:     logs,
		Total:    len(logs),
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "render error: %v", err)
	}
}

// traceLogsJSON returns JSON for trace logs.
func (rt *Router) traceLogsJSON(c *gin.Context) {
	traceId := ginx.UrlParamStr(c, "traceid")
	if !loggrep.IsValidTraceID(traceId) {
		ginx.Bomb(200, "invalid trace id format")
	}

	logs, instance, err := rt.getTraceLogs(traceId)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(loggrep.EventDetailResp{
		Logs:     logs,
		Instance: instance,
	}, nil)
}

// getTraceLogs finds the same-engine instances and queries each one
// until trace logs are found. Trace logs belong to a single instance.
func (rt *Router) getTraceLogs(traceId string) ([]string, string, error) {
	keyword := "trace_id=" + traceId
	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)
	engineName := rt.Alert.Heartbeat.EngineName

	// try local first
	logs, err := loggrep.GrepLatestLogFiles(rt.LogDir, keyword)
	if err == nil && len(logs) > 0 {
		return logs, instance, nil
	}

	// find all instances with the same engineName
	servers, err := models.AlertingEngineGetsInstances(rt.Ctx,
		"engine_cluster = ? and clock > ?",
		engineName, time.Now().Unix()-30)
	if err != nil {
		return nil, "", err
	}

	// loop through remote instances until we find logs
	for _, node := range servers {
		if node == instance {
			continue // already tried local
		}

		logs, nodeAddr, err := rt.forwardTraceLogs(node, traceId)
		if err != nil {
			continue
		}
		if len(logs) > 0 {
			return logs, nodeAddr, nil
		}
	}

	// no logs found on any instance
	return nil, instance, nil
}

func (rt *Router) forwardTraceLogs(node, traceId string) ([]string, string, error) {
	url := fmt.Sprintf("http://%s/v1/n9e/trace-logs/%s", node, traceId)
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
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
