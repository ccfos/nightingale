package router

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"
	"github.com/toolkits/pkg/logger"

	"github.com/gin-gonic/gin"
)

// traceLogsPage renders the HTML log viewer shell for trace logs. Like
// eventDetailPage, the shell carries no log data; the login check happens on
// the /logs sibling route its JS fetches.
func (rt *Router) traceLogsPage(c *gin.Context) {
	traceId := ginx.UrlParamStr(c, "traceid")
	if !loggrep.IsValidTraceID(traceId) {
		c.String(http.StatusBadRequest, "invalid trace id format")
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	err := loggrep.RenderTraceLogsHTML(c.Writer, loggrep.TraceLogsPageData{TraceID: traceId})
	if err != nil {
		c.String(http.StatusInternalServerError, "render error: %v", err)
	}
}

// traceLogsJSON returns trace logs; the route requires a login. Trace logs
// span rules and busi groups, so there is no single group to check against.
func (rt *Router) traceLogsJSON(c *gin.Context) {
	traceId := ginx.UrlParamStr(c, "traceid")
	if !loggrep.IsValidTraceID(traceId) {
		ginx.Bomb(200, "invalid trace id format")
	}

	resp, err := rt.getTraceLogs(c.Request.Context(), traceId)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(resp, nil)
}

// getTraceLogs finds the same-engine instances and queries each one
// until trace logs are found. Trace logs belong to a single instance.
func (rt *Router) getTraceLogs(ctx context.Context, traceId string) (loggrep.EventDetailResp, error) {
	keyword := "trace_id=" + traceId
	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)
	engineName := rt.Alert.Heartbeat.EngineName

	// every instance may have to be asked before the owning one is found, so
	// they share one budget rather than each getting the full one
	deadline := time.Now().Add(loggrep.DefaultTimeout)

	// A search that did not finish must say so even when it found nothing:
	// otherwise an aborted scan and a genuine "this trace was never logged
	// here" are indistinguishable, and the page reports the second one.
	// These carry that across every instance that gets asked.
	var truncated bool
	var reason string

	note := func(t bool, r string) {
		if !t {
			return
		}
		truncated = true
		if reason == "" {
			reason = r
		}
	}

	// try local first
	res := loggrep.GrepLatestLogFiles(ctx, rt.LogDir, keyword, loggrep.GrepOptions{Deadline: deadline})
	if len(res.Logs) > 0 {
		return loggrep.EventDetailResp{
			Logs:      res.Logs,
			Instance:  instance,
			Truncated: res.Truncated,
			Reason:    res.Reason,
		}, nil
	}
	note(res.Truncated, res.Reason)

	// find all instances with the same engineName
	servers, err := models.AlertingEngineGetsInstances(rt.Ctx,
		"engine_cluster = ? and clock > ?",
		engineName, time.Now().Unix()-30)
	if err != nil {
		return loggrep.EventDetailResp{}, err
	}

	// loop through remote instances until we find logs
	for _, node := range servers {
		if node == instance {
			continue // already tried local
		}

		if time.Now().After(deadline) {
			note(true, loggrep.ReasonTimeout)
			break
		}

		resp, err := rt.forwardLogQuery(ctx, node, "/trace-logs/"+traceId, time.Time{}, time.Until(deadline))
		if err != nil {
			// an instance that could not be asked leaves a hole in the answer
			logger.Errorf("forwardTraceLogs failed: %v", err)
			note(true, loggrep.ReasonTimeout)
			continue
		}
		if len(resp.Logs) > 0 {
			return resp, nil
		}
		note(resp.Truncated, resp.Reason)
	}

	return loggrep.EventDetailResp{
		Instance:  instance,
		Truncated: truncated,
		Reason:    reason,
	}, nil
}
