package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// alertEvalDetailPage renders an HTML log viewer page for alert rule evaluation logs.
func (rt *Router) alertEvalDetailPage(c *gin.Context) {
	id := ginx.UrlParamStr(c, "id")
	if !loggrep.IsValidRuleID(id) {
		c.String(http.StatusBadRequest, "invalid rule id format")
		return
	}

	logs, instance, err := rt.getAlertEvalLogs(id)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	err = loggrep.RenderAlertEvalHTML(c.Writer, loggrep.AlertEvalPageData{
		RuleID:   id,
		Instance: instance,
		Logs:     logs,
		Total:    len(logs),
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "render error: %v", err)
	}
}

// alertEvalDetailJSON returns JSON for alert rule evaluation logs.
func (rt *Router) alertEvalDetailJSON(c *gin.Context) {
	id := ginx.UrlParamStr(c, "id")
	if !loggrep.IsValidRuleID(id) {
		ginx.Bomb(200, "invalid rule id format")
	}

	logs, instance, err := rt.getAlertEvalLogs(id)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(loggrep.EventDetailResp{
		Logs:     logs,
		Instance: instance,
	}, nil)
}

// getAlertEvalLogs resolves the target instance(s) and retrieves alert eval logs.
func (rt *Router) getAlertEvalLogs(id string) ([]string, string, error) {
	ruleId, _ := strconv.ParseInt(id, 10, 64)
	rule, err := models.AlertRuleGetById(rt.Ctx, ruleId)
	if err != nil {
		return nil, "", err
	}
	if rule == nil {
		return nil, "", fmt.Errorf("no such alert rule")
	}

	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)
	keyword := fmt.Sprintf("alert_eval_%s", id)

	// Get datasource IDs for this rule
	dsIds := rt.DatasourceCache.GetIDsByDsCateAndQueries(rule.Cate, rule.DatasourceQueries)
	if len(dsIds) == 0 {
		// No datasources found (e.g. host rule), try local grep
		logs, err := loggrep.GrepLogDir(rt.LogDir, keyword)
		return logs, instance, err
	}

	// Find unique target nodes via hash ring
	nodeSet := make(map[string]struct{})
	for _, dsId := range dsIds {
		node, err := naming.DatasourceHashRing.GetNode(strconv.FormatInt(dsId, 10), id)
		if err != nil {
			continue
		}
		nodeSet[node] = struct{}{}
	}

	if len(nodeSet) == 0 {
		// Hash ring not ready, grep locally
		logs, err := loggrep.GrepLogDir(rt.LogDir, keyword)
		return logs, instance, err
	}

	// Collect logs from all target nodes
	var allLogs []string
	var instances []string

	for node := range nodeSet {
		if node == instance {
			logs, err := loggrep.GrepLogDir(rt.LogDir, keyword)
			if err == nil {
				allLogs = append(allLogs, logs...)
				instances = append(instances, node)
			}
		} else {
			logs, nodeAddr, err := rt.forwardAlertEvalDetail(node, id)
			if err == nil {
				allLogs = append(allLogs, logs...)
				instances = append(instances, nodeAddr)
			}
		}
	}

	// Sort logs by timestamp descending
	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i] > allLogs[j]
	})

	if len(allLogs) > loggrep.MaxLogLines {
		allLogs = allLogs[:loggrep.MaxLogLines]
	}

	return allLogs, strings.Join(instances, ", "), nil
}

func (rt *Router) forwardAlertEvalDetail(node, id string) ([]string, string, error) {
	url := fmt.Sprintf("http://%s/v1/n9e/alert-eval-detail/%s", node, id)
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
