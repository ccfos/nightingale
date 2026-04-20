package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

const FlashDutyIdent = "flashduty"

type FlashDutyProvider struct{}

func (p *FlashDutyProvider) Ident() string {
	return FlashDutyIdent
}

func (p *FlashDutyProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != FlashDutyIdent {
		return errors.New("flashduty provider requires request_type: flashduty")
	}
	return config.ValidateFlashDutyRequestConfig()
}

func (p *FlashDutyProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req.Config.RequestConfig == nil || req.Config.RequestConfig.FlashDutyRequestConfig == nil {
		return &NotifyResult{Target: "", Response: "", Err: errors.New("flashduty request config not found")}
	}
	channelIDs := req.FlashDutyChannelIDs
	if len(channelIDs) == 0 {
		channelIDs = []int64{0}
	}
	var responses []string
	var targets []string
	var failedMsgs []string
	for _, channelID := range channelIDs {
		target := strconv.FormatInt(channelID, 10)
		targets = append(targets, target)
		resp, err := SendFlashDuty(req.Config.RequestConfig.FlashDutyRequestConfig, req.Events, channelID, req.HttpClient)
		if err != nil {
			// 单个 channel 失败不要中断其他 channel 的发送
			failedMsgs = append(failedMsgs, fmt.Sprintf("channel %s: %v", target, err))
			responses = append(responses, fmt.Sprintf("channel %s: %s", target, resp))
			continue
		}
		responses = append(responses, resp)
	}
	var aggErr error
	if len(failedMsgs) > 0 {
		aggErr = errors.New(strings.Join(failedMsgs, " | "))
	}
	return &NotifyResult{Target: strings.Join(targets, ","), Response: strings.Join(responses, "; "), Err: aggErr}
}

func SendFlashDuty(config *models.FlashDutyRequestConfig, events []*models.AlertCurEvent, flashDutyChannelID int64, client *http.Client) (string, error) {
	// todo 每一个 channel 批量发送事件
	if client == nil {
		return "", fmt.Errorf("http client not found")
	}

	body, err := json.Marshal(events)
	if err != nil {
		return "", err
	}

	url := config.IntegrationUrl

	retrySleep := time.Second
	if config.RetrySleep > 0 {
		retrySleep = time.Duration(config.RetrySleep) * time.Millisecond
	}

	retryTimes := 3
	if config.RetryTimes > 0 {
		retryTimes = config.RetryTimes
	}

	// 把最后一次错误保存下来，后面返回，让用户在页面上也可以看到
	var lastErrorMessage string
	for i := 0; i <= retryTimes; i++ {
		req, err := makeFlashDutyRequest(url, body, flashDutyChannelID)
		if err != nil {
			logger.Errorf("send_flashduty: failed to create request. url=%s request_body=%s error=%v", url, string(body), err)
			return fmt.Sprintf("failed to create request. error: %v", err), err
		}

		// 直接使用客户端发送请求，超时时间已经在 client 中设置
		resp, err := client.Do(req)
		if err != nil {
			logger.Errorf("send_flashduty: http_call=fail url=%s request_body=%s error=%v times=%d", url, string(body), err, i+1)
			if i < retryTimes {
				// 重试等待时间，后面要放到页面上配置
				time.Sleep(retrySleep)
			}
			lastErrorMessage = err.Error()
			continue
		}

		// 走到这里，说明请求 Flashduty 成功，不管 Flashduty 返回了什么结果，都不判断，仅保存，给用户查看即可
		// 比如服务端返回 5xx，也不要重试，重试可能会导致服务端数据有问题。告警事件这样的东西，没有那么关键，只要最终能在 UI 上看到调用结果就行
		var resBody []byte
		if resp.Body != nil {
			defer resp.Body.Close()

			resBody, err = io.ReadAll(resp.Body)
			if err != nil {
				logger.Errorf("send_flashduty: failed to read response. request_body=%s, error=%v", string(body), err)
				resBody = []byte("failed to read response. error: " + err.Error())
			}
		}

		logger.Infof("send_flashduty: http_call=succ url=%s request_body=%s response_code=%d response_body=%s times=%d", url, string(body), resp.StatusCode, string(resBody), i+1)
		return fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(resBody)), nil
	}

	return lastErrorMessage, errors.New("failed to send request")
}

func makeFlashDutyRequest(url string, bodyBytes []byte, flashDutyChannelID int64) (*http.Request, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	// 设置 URL 参数
	query := req.URL.Query()
	if flashDutyChannelID != 0 {
		// 如果 flashduty 有配置协作空间(channel_id)，则传入 channel_id 参数
		query.Add("channel_id", strconv.FormatInt(flashDutyChannelID, 10))
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Add("Content-Type", "application/json")
	return req, nil
}
