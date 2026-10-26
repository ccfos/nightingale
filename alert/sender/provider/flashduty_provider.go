package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// Prepare private event copies so other notification channels can still use the screenshots.
func prepareFlashDutyEvents(config *models.FlashDutyRequestConfig, events []*models.AlertCurEvent, client *http.Client) ([]*models.AlertCurEvent, error) {
	if client == nil {
		return nil, errors.New("http client not found")
	}
	prepared := make([]*models.AlertCurEvent, len(events))
	var uploads sync.WaitGroup
	// Bound concurrent uploads to avoid excessive memory usage for large batches.
	uploadSlots := make(chan struct{}, 5)
	for i, event := range events {
		if event == nil {
			continue
		}
		copied := *event
		copied.ShotImageBase64 = nil
		prepared[i] = &copied
		// The snapshot field holds one image. Pick it consistently when multiple screenshots exist.
		keys := make([]string, 0, len(event.ShotImageBase64))
		for key := range event.ShotImageBase64 {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			imageBase64 := event.ShotImageBase64[key]
			if strings.TrimSpace(imageBase64) == "" {
				continue
			}
			var ruleConfig map[string]json.RawMessage
			configJSON := []byte(event.RuleConfig)
			if event.RuleConfigJson != nil {
				var err error
				configJSON, err = json.Marshal(event.RuleConfigJson)
				if err != nil {
					logger.Warningf("send_flashduty: skip image attachment event_id=%d: marshal rule config: %v", event.Id, err)
					break
				}
			}
			if len(configJSON) > 0 {
				if err := json.Unmarshal(configJSON, &ruleConfig); err != nil {
					logger.Warningf("send_flashduty: skip image attachment event_id=%d: parse rule config: %v", event.Id, err)
					break
				}
			}
			if ruleConfig == nil {
				ruleConfig = make(map[string]json.RawMessage)
			}
			uploadSlots <- struct{}{}
			uploads.Add(1)
			go func() {
				defer uploads.Done()
				defer func() { <-uploadSlots }()
				imageKey, err := uploadFlashDutyImage(config.IntegrationUrl, imageBase64, client)
				if err != nil {
					// Screenshots are optional: keep sending alerts even if uploading fails.
					logger.Warningf("send_flashduty: upload image failed event_id=%d: %v", event.Id, err)
					return
				}
				// Match NewNorthstarAlertEvent: annotations.img_key and a string snapshot_pic_url.
				ruleConfig["snapshot_pic_url"], _ = json.Marshal(imageKey)
				copied.RuleConfigJson = ruleConfig
				copied.AnnotationsJSON = make(map[string]string, len(event.AnnotationsJSON)+1)
				for name, value := range event.AnnotationsJSON {
					copied.AnnotationsJSON[name] = value
				}
				copied.AnnotationsJSON["img_key"] = imageKey
			}()
			break
		}
	}
	uploads.Wait()
	return prepared, nil
}

func uploadFlashDutyImage(integrationURL, imageBase64 string, client *http.Client) (string, error) {
	uploadURL, err := url.Parse(integrationURL)
	if err != nil {
		return "", err
	}
	query := url.Values{"integration_key": {uploadURL.Query().Get("integration_key")}}
	uploadURL.Path = "/push/image/upload"
	uploadURL.RawPath = ""
	uploadURL.RawQuery = query.Encode()
	uploadURL.Fragment = ""
	imgBytes, err := decodeBase64Payload(imageBase64)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}
	if len(imgBytes) == 0 || len(imgBytes) > 5*1024*1024 {
		return "", errors.New("image must be non-empty and no larger than 5MB")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "snapshot.png")
	if err != nil {
		return "", err
	}
	if _, err = part.Write(imgBytes); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, uploadURL.String(), &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("upload image returned status %d", resp.StatusCode)
	}
	var result struct {
		Data struct {
			ImageKey string `json:"image_key"`
		} `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse upload image response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("upload image failed: %s: %s", result.Error.Code, result.Error.Message)
	}
	if result.Data.ImageKey == "" {
		return "", errors.New("upload image response has no image_key")
	}
	return result.Data.ImageKey, nil
}

func SendFlashDuty(config *models.FlashDutyRequestConfig, events []*models.AlertCurEvent, flashDutyChannelID int64, client *http.Client) (string, error) {
	events, err := prepareFlashDutyEvents(config, events, client)
	if err != nil {
		return "", err
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
