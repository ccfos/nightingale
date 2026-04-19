package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

const PagerDutyIdent = "pagerduty"

type PagerDutyProvider struct{}

func (p *PagerDutyProvider) Ident() string {
	return PagerDutyIdent
}

func (p *PagerDutyProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != PagerDutyIdent {
		return errors.New("pagerduty provider requires request_type: pagerduty")
	}
	return config.ValidatePagerDutyRequestConfig()
}

func (p *PagerDutyProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req.Config.RequestConfig == nil || req.Config.RequestConfig.PagerDutyRequestConfig == nil {
		return &NotifyResult{Target: "", Response: "", Err: errors.New("pagerduty request config not found")}
	}

	if len(req.PagerDutyRoutingKeys) == 0 {
		return &NotifyResult{Target: "", Response: "", Err: errors.New("pagerduty requires at least one routing key in sendtos")}
	}
	var responses []string
	var failedMsgs []string
	for _, routingKey := range req.PagerDutyRoutingKeys {
		resp, err := SendPagerDuty(req.Config.RequestConfig.PagerDutyRequestConfig, req.Events, routingKey, req.SiteUrl, req.HttpClient)
		if err != nil {
			// 单个 routing key 失败不要中断其他 routing key 的发送
			failedMsgs = append(failedMsgs, fmt.Sprintf("routing_key %s: %v", routingKey, err))
			responses = append(responses, fmt.Sprintf("routing_key %s: %s", routingKey, resp))
			continue
		}
		responses = append(responses, resp)
	}
	var aggErr error
	if len(failedMsgs) > 0 {
		aggErr = errors.New(strings.Join(failedMsgs, " | "))
	}
	return &NotifyResult{Target: strings.Join(req.PagerDutyRoutingKeys, ","), Response: strings.Join(responses, "; "), Err: aggErr}
}

func SendPagerDuty(config *models.PagerDutyRequestConfig, events []*models.AlertCurEvent, routingKey, siteUrl string, client *http.Client) (string, error) {
	if client == nil {
		return "", fmt.Errorf("http client not found")
	}
	if config == nil {
		return "", fmt.Errorf("pagerduty request config not found")
	}

	retrySleep := time.Second
	if config.RetrySleep > 0 {
		retrySleep = time.Duration(config.RetrySleep) * time.Millisecond
	}

	retryTimes := 3
	if config.RetryTimes > 0 {
		retryTimes = config.RetryTimes
	}

	endpoint := "https://events.pagerduty.com/v2/enqueue"
	var failedMsgs []string
	var responses []string

	for _, event := range events {
		action := "trigger"
		if event.IsRecovered {
			action = "resolve"
		}

		severity := "critical"
		switch event.Severity {
		case 2:
			severity = "error"
		case 3:
			severity = "warning"
		}

		jsonBody := map[string]interface{}{
			"routing_key":  routingKey,
			"event_action": action,
			"dedup_key":    event.Hash,
			"payload": map[string]interface{}{
				"summary":   event.RuleName,
				"source":    event.Cluster,
				"severity":  severity,
				"group":     event.GroupName,
				"component": event.Target,
				"timestamp": time.Unix(event.TriggerTime, 0).Format(time.RFC3339),
				"custom_details": map[string]interface{}{
					"tags":               event.TagsJSON,
					"annotations":        event.AnnotationsJSON,
					"cluster":            event.Cluster,
					"rule_id":            event.RuleId,
					"rule_note":          event.RuleNote,
					"rule_prod":          event.RuleProd,
					"prom_ql":            event.PromQl,
					"target_ident":       event.TargetIdent,
					"target_note":        event.TargetNote,
					"datasource_id":      event.DatasourceId,
					"first_trigger_time": time.Unix(event.FirstTriggerTime, 0).Format(time.RFC3339),
					"prom_for_duration":  event.PromForDuration,
					"runbook_url":        event.RunbookUrl,
					"notify_cur_number":  event.NotifyCurNumber,
					"group_id":           event.GroupId,
					"cate":               event.Cate,
				},
			},
			"links": []map[string]string{
				{"href": fmt.Sprintf("%s/alert-his-events/%d", siteUrl, event.Id), "text": "Event Detail"},
				{"href": fmt.Sprintf("%s/alert-mutes/add?__event_id=%d", siteUrl, event.Id), "text": "Mute this alert"},
			},
		}

		body, err := json.Marshal(jsonBody)
		if err != nil {
			logger.Errorf("send_pagerduty: failed to marshal request body. error=%v", err)
			failedMsgs = append(failedMsgs, fmt.Sprintf("event %d marshal error: %v", event.Id, err))
			// 记录一条空响应占位，方便上层区分事件
			responses = append(responses, fmt.Sprintf("event %d: marshal error: %v", event.Id, err))
			continue
		}

		var lastErrorMessage string
		var lastRespSummary string
		attempts := retryTimes + 1
		for i := 0; i < attempts; i++ {
			req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
			if err != nil {
				logger.Errorf("send_pagerduty: failed to create request. url=%s request_body=%s error=%v", endpoint, string(body), err)
				lastErrorMessage = err.Error()
				if i < attempts-1 {
					time.Sleep(retrySleep)
					continue
				}
				break
			}
			req.Header.Add("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				logger.Errorf("send_pagerduty: http_call=fail url=%s request_body=%s error=%v times=%d", endpoint, string(body), err, i+1)
				lastErrorMessage = err.Error()
				if i < attempts-1 {
					time.Sleep(retrySleep)
					continue
				}
				break
			}

			// 确保关闭 body
			var resBody []byte
			if resp.Body != nil {
				resBody, err = io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					logger.Errorf("send_pagerduty: failed to read response. request_body=%s, error=%v", string(body), err)
					resBody = []byte("failed to read response. error: " + err.Error())
				}
			} else {
				resBody = []byte("")
			}

			respSummary := fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(resBody))
			lastRespSummary = respSummary

			logger.Infof("send_pagerduty: http_call=succ url=%s request_body=%s response_code=%d response_body=%s times=%d", endpoint, string(body), resp.StatusCode, string(resBody), i+1)

			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
				// 当前事件发送成功
				lastErrorMessage = ""
				break
			}

			lastErrorMessage = respSummary
			if i < attempts-1 {
				time.Sleep(retrySleep)
				continue
			}
			break
		}

		// 保存本次事件的响应摘要（无论成功或失败），便于上层记录 traceId 等信息
		if lastRespSummary == "" && lastErrorMessage != "" {
			lastRespSummary = lastErrorMessage
		}
		responses = append(responses, fmt.Sprintf("event %d: %s", event.Id, lastRespSummary))

		if lastErrorMessage != "" {
			failedMsgs = append(failedMsgs, fmt.Sprintf("event %d: %s", event.Id, lastErrorMessage))
		}
	}

	// 将每个 event 的响应摘要返回给上层，便于记录 pagerduty 返回的 traceId 等信息
	if len(failedMsgs) > 0 {
		return strings.Join(responses, " | "), errors.New(strings.Join(failedMsgs, " | "))
	}
	return strings.Join(responses, " | "), nil
}
