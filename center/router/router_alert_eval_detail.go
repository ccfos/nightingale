package router

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

// alertEvalDetailPage renders the HTML log viewer shell for alert rule
// evaluation logs. Like eventDetailPage, the shell carries no log data; the
// permission check happens on the /logs sibling route its JS fetches.
func (rt *Router) alertEvalDetailPage(c *gin.Context) {
	id := ginx.UrlParamStr(c, "id")
	if !loggrep.IsValidRuleID(id) {
		c.String(http.StatusBadRequest, "invalid rule id format")
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	err := loggrep.RenderAlertEvalHTML(c.Writer, loggrep.AlertEvalPageData{RuleID: id})
	if err != nil {
		c.String(http.StatusInternalServerError, "render error: %v", err)
	}
}

// alertEvalDetailJSON returns alert rule evaluation logs; requires a login
// and read permission on the busi group the rule belongs to.
func (rt *Router) alertEvalDetailJSON(c *gin.Context) {
	id := ginx.UrlParamStr(c, "id")
	if !loggrep.IsValidRuleID(id) {
		ginx.Bomb(200, "invalid rule id format")
	}

	ruleId, _ := strconv.ParseInt(id, 10, 64)
	rule, err := models.AlertRuleGetById(rt.Ctx, ruleId)
	ginx.Dangerous(err)
	if rule == nil {
		ginx.Bomb(404, "no such alert rule")
	}

	rt.bgroCheckAllowMissing(c, rule.GroupId)

	resp, err := rt.getAlertEvalLogsByRule(c.Request.Context(), rule, id)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(resp, nil)
}

// getAlertEvalLogs resolves the target instance(s) and retrieves alert eval
// logs. It keeps the flat shape the aiagent troubleshooting tool binds to, and
// passes the truncation reason out with it - see getEventLogs for why the
// model must not be handed a partial result that looks complete.
func (rt *Router) getAlertEvalLogs(id string) ([]string, string, string, error) {
	ruleId, _ := strconv.ParseInt(id, 10, 64)
	rule, err := models.AlertRuleGetById(rt.Ctx, ruleId)
	if err != nil {
		return nil, "", "", err
	}
	if rule == nil {
		return nil, "", "", fmt.Errorf("no such alert rule")
	}

	resp, err := rt.getAlertEvalLogsByRule(context.Background(), rule, id)
	if err != nil {
		return nil, "", "", err
	}

	return resp.Logs, resp.Instance, truncatedReason(resp), nil
}

func (rt *Router) getAlertEvalLogsByRule(ctx context.Context, rule *models.AlertRule, id string) (loggrep.EventDetailResp, error) {
	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)
	keyword := fmt.Sprintf("alert_eval_%s", id)

	localGrep := func() (loggrep.EventDetailResp, error) {
		res := loggrep.GrepLogDir(ctx, rt.LogDir, keyword, loggrep.GrepOptions{})
		return loggrep.EventDetailResp{
			Logs:      res.Logs,
			Instance:  instance,
			Truncated: res.Truncated,
			Reason:    res.Reason,
		}, nil
	}

	// Get datasource IDs for this rule
	dsIds := rt.DatasourceCache.GetIDsByDsCateAndQueries(rule.Cate, rule.DatasourceQueries)
	if len(dsIds) == 0 {
		// No datasources found (e.g. host rule), try local grep
		return localGrep()
	}

	// Find unique target nodes via hash ring, with DB fallback
	nodeSet := make(map[string]struct{})
	for _, dsId := range dsIds {
		node, err := rt.getNodeForDatasource(dsId, id)
		if err != nil {
			continue
		}
		nodeSet[node] = struct{}{}
	}

	if len(nodeSet) == 0 {
		// Hash ring not ready, grep locally
		return localGrep()
	}

	// The nodes are queried one after another, so they share a single budget
	// instead of each getting the full one: with several engine instances the
	// per-node budgets would otherwise add up past any proxy read timeout.
	deadline := time.Now().Add(loggrep.DefaultTimeout)

	var allLogs []string
	var instances []string
	var truncated bool
	var reason string

	for node := range nodeSet {
		var resp loggrep.EventDetailResp
		var err error

		if node == instance {
			res := loggrep.GrepLogDir(ctx, rt.LogDir, keyword, loggrep.GrepOptions{Deadline: deadline})
			resp = loggrep.EventDetailResp{
				Logs:      res.Logs,
				Instance:  node,
				Truncated: res.Truncated,
				Reason:    res.Reason,
			}
		} else {
			resp, err = rt.forwardLogQuery(ctx, node, "/alert-eval-detail/"+id, time.Time{}, time.Until(deadline))
		}

		if err != nil {
			// a node that could not be reached leaves a hole in the result,
			// which is exactly what the truncated flag is for
			logger.Warningf("alert-eval-detail: query node %s failed: %v", node, err)
			truncated = true
			if reason == "" {
				reason = loggrep.ReasonTimeout
			}
			continue
		}

		allLogs = append(allLogs, resp.Logs...)
		instances = append(instances, resp.Instance)
		if resp.Truncated {
			truncated = true
			if reason == "" || resp.Reason == loggrep.ReasonTimeout {
				reason = resp.Reason
			}
		}
	}

	// Sort logs by timestamp descending
	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i] > allLogs[j]
	})

	if len(allLogs) > loggrep.MaxLogLines {
		allLogs = allLogs[:loggrep.MaxLogLines]
		truncated = true
		if reason == "" {
			reason = loggrep.ReasonLimit
		}
	}

	return loggrep.EventDetailResp{
		Logs:      allLogs,
		Instance:  strings.Join(instances, ", "),
		Truncated: truncated,
		Reason:    reason,
	}, nil
}
