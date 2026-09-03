package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
)

// eventDetailPage renders the HTML log viewer shell (for pages group). The
// shell carries no log data: its JS fetches the /logs sibling route with the
// login JWT the UI keeps in localStorage, and the permission check happens
// there. That is why the shell itself needs no authentication even though it
// is opened via a plain link in a new browser tab.
func (rt *Router) eventDetailPage(c *gin.Context) {
	hash := ginx.UrlParamStr(c, "hash")
	if !loggrep.IsValidHash(hash) {
		c.String(http.StatusBadRequest, "invalid hash format")
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	err := loggrep.RenderHTML(c.Writer, loggrep.PageData{Hash: hash})
	if err != nil {
		c.String(http.StatusInternalServerError, "render error: %v", err)
	}
}

// eventDetailJSON returns the processing logs of an event. The logs may
// contain sensitive content such as notify channel credentials, so knowing
// the hash alone must not grant access: the route requires a login and this
// handler checks read permission on the busi group the event belongs to.
func (rt *Router) eventDetailJSON(c *gin.Context) {
	hash := ginx.UrlParamStr(c, "hash")
	if !loggrep.IsValidHash(hash) {
		ginx.Bomb(200, "invalid hash format")
	}

	event, err := models.AlertHisEventGetByHash(rt.Ctx, hash)
	ginx.Dangerous(err)
	if event == nil {
		ginx.Bomb(404, "no such alert event")
	}

	// history events outlive their busi group, so a group that is gone must
	// not turn into a 404 that reads like "no such event"
	rt.bgroCheckAllowMissing(c, event.GroupId)

	resp, err := rt.getEventLogsByEvent(c.Request.Context(), event)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(resp, nil)
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

// getEventLogs resolves the target instance and retrieves logs. It keeps the
// flat shape the aiagent troubleshooting tool binds to, and passes the
// truncation reason out with it: the search runs on a time budget, so an empty
// or short result is not proof that nothing was logged, and the model has to
// be told which of the two it is looking at.
func (rt *Router) getEventLogs(hash string) ([]string, string, string, error) {
	event, err := models.AlertHisEventGetByHash(rt.Ctx, hash)
	if err != nil {
		return nil, "", "", err
	}
	if event == nil {
		return nil, "", "", fmt.Errorf("no such alert event")
	}

	resp, err := rt.getEventLogsByEvent(context.Background(), event)
	if err != nil {
		return nil, "", "", err
	}

	return resp.Logs, resp.Instance, truncatedReason(resp), nil
}

// truncatedReason turns the response flags into the reason string the aiagent
// tools carry, empty when the result is known to be complete.
func truncatedReason(resp loggrep.EventDetailResp) string {
	if !resp.Truncated {
		return ""
	}
	if resp.Reason == "" {
		return loggrep.ReasonTimeout
	}
	return resp.Reason
}

// eventLogsSince is the point before which log files cannot mention the event,
// and so need not be opened at all. The slack absorbs clock skew between the
// database timestamps and the log host, plus the evaluation logs written in
// the cycle that produced the event.
func eventLogsSince(event *models.AlertHisEvent) time.Time {
	ts := event.FirstTriggerTime
	if ts == 0 || (event.TriggerTime > 0 && event.TriggerTime < ts) {
		ts = event.TriggerTime
	}
	if ts == 0 {
		return time.Time{}
	}

	return time.Unix(ts, 0).Add(-time.Hour)
}

func (rt *Router) getEventLogsByEvent(ctx context.Context, event *models.AlertHisEvent) (loggrep.EventDetailResp, error) {
	ruleId := strconv.FormatInt(event.RuleId, 10)

	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)

	node, err := rt.getNodeForDatasource(event.DatasourceId, ruleId)
	if err != nil || node == instance {
		// hashring not ready or target is self, handle locally
		res := loggrep.GrepLogDir(ctx, rt.LogDir, event.Hash, loggrep.GrepOptions{
			Since: eventLogsSince(event),
		})
		return loggrep.EventDetailResp{
			Logs:      res.Logs,
			Instance:  instance,
			Truncated: res.Truncated,
			Reason:    res.Reason,
		}, nil
	}

	// forward to the target alert instance
	return rt.forwardEventDetail(ctx, node, event.Hash, eventLogsSince(event))
}

// forwardLogQuery calls a log endpoint on another engine instance. The budget
// is the wall clock the remote side gets: it is passed on as a query parameter
// so the remote grep stops on the same deadline instead of running on after
// this side has already given up.
func (rt *Router) forwardLogQuery(ctx context.Context, node, path string, since time.Time, budget time.Duration) (loggrep.EventDetailResp, error) {
	out := loggrep.EventDetailResp{Instance: node}

	if budget <= 0 {
		return out, fmt.Errorf("no time left to query %s", node)
	}

	url := fmt.Sprintf("http://%s/v1/n9e%s?timeout=%d", node, path, int64(budget/time.Millisecond))
	if !since.IsZero() {
		url = fmt.Sprintf("%s&since=%d", url, since.Unix())
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return out, err
	}

	for user, pass := range rt.HTTP.APIForService.BasicAuth {
		req.SetBasicAuth(user, pass)
		break
	}

	// the remote is expected to answer within its own budget; allow a margin
	// for connect and transfer on top of it
	client := &http.Client{Timeout: budget + 5*time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return out, fmt.Errorf("forward to %s failed: %v", node, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return out, err
	}

	var result struct {
		Dat loggrep.EventDetailResp `json:"dat"`
		Err string                  `json:"err"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return out, err
	}
	if result.Err != "" {
		return out, fmt.Errorf("%s", result.Err)
	}

	if result.Dat.Instance == "" {
		result.Dat.Instance = node
	}

	return result.Dat, nil
}

func (rt *Router) forwardEventDetail(ctx context.Context, node, hash string, since time.Time) (loggrep.EventDetailResp, error) {
	return rt.forwardLogQuery(ctx, node, "/event-detail/"+hash, since, loggrep.DefaultTimeout)
}
