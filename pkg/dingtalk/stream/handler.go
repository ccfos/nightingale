//go:build ignore
// +build ignore

// TODO(dingtalkapp): 钉钉应用本次不上线，Stream handler 先 build tag 屏蔽；上线时删除顶部两行即可恢复。

package stream

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/models"
	nctx "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
	"github.com/toolkits/pkg/logger"
)

const (
	eventTypeCoolAppInstall   = "im_cool_app_install"
	eventTypeCoolAppUninstall = "im_cool_app_uninstall"
)

type coolAppEventData struct {
	EventID                string `json:"eventId"`
	OpenConversationID     string `json:"openConversationId"`
	OpenConversationCorpID string `json:"openConversationCorpId"`
	CoolAppCode            string `json:"coolAppCode"`
	RobotCode              string `json:"robotCode"`
	Operator               string `json:"operator"`
}

// EventHandlerDeps 处理 install/uninstall 所需依赖。
type EventHandlerDeps struct {
	Nctx       *nctx.Context
	ClientID   string
	AppSecret  string
	HTTPClient *http.Client
}

type eventProcessor struct {
	EventHandlerDeps
}

func newEventProcessor(d EventHandlerDeps) *eventProcessor {
	return &eventProcessor{EventHandlerDeps: d}
}

func (p *eventProcessor) onDataFrame(c context.Context, df *payload.DataFrame) (*payload.DataFrameResponse, error) {
	hdr := event.NewEventHeaderFromDataFrame(df)
	switch hdr.EventType {
	case eventTypeCoolAppInstall:
		p.handleInstall(c, hdr, df.Data)
	case eventTypeCoolAppUninstall:
		p.handleUninstall(c, hdr, df.Data)
	default:
		logger.Debugf("dingtalk stream ignore eventType=%s eventId=%s", hdr.EventType, hdr.EventId)
	}
	return event.NewSuccessResponse()
}

func (p *eventProcessor) handleInstall(stdCtx context.Context, hdr *event.EventHeader, dataJSON string) {
	var raw coolAppEventData
	if err := json.Unmarshal([]byte(dataJSON), &raw); err != nil {
		logger.Warningf("dingtalk install parse data: %v", err)
		return
	}
	openID := strings.TrimSpace(raw.OpenConversationID)
	if openID == "" {
		logger.Warningf("dingtalk install missing openConversationId eventId=%s", hdr.EventId)
		return
	}
	if p.Nctx == nil || p.Nctx.DB == nil {
		return
	}

	row := &models.DingtalkGroup{
		ClientID:               p.ClientID,
		OpenConversationCorpID: strings.TrimSpace(raw.OpenConversationCorpID),
		OpenConversationID:     openID,
		CoolAppCode:            raw.CoolAppCode,
		RobotCode:              raw.RobotCode,
	}

	if p.HTTPClient != nil && p.AppSecret != "" {
		token, err := provider.GetAccessToken(stdCtx, p.HTTPClient, p.ClientID, p.AppSecret)
		if err != nil {
			logger.Warningf("dingtalk install get token: %v", err)
		} else {
			info, err := provider.GetScenarioGroupInfo(stdCtx, p.HTTPClient, token, openID)
			if err != nil {
				logger.Warningf("dingtalk install GetScenarioGroupInfo: %v", err)
			} else if info != nil {
				row.Title = info.Title
			}
		}
	}

	if err := models.UpsertDingtalkGroupInstall(p.Nctx, row); err != nil {
		logger.Errorf("dingtalk install upsert db: %v", err)
	}
}

func (p *eventProcessor) handleUninstall(stdCtx context.Context, hdr *event.EventHeader, dataJSON string) {
	var raw coolAppEventData
	if err := json.Unmarshal([]byte(dataJSON), &raw); err != nil {
		logger.Warningf("dingtalk uninstall parse data: %v", err)
		return
	}
	openID := strings.TrimSpace(raw.OpenConversationID)
	if openID == "" {
		return
	}
	if p.Nctx == nil || p.Nctx.DB == nil {
		return
	}
	corp := strings.TrimSpace(raw.OpenConversationCorpID)
	if err := models.MarkDingtalkGroupUninstall(p.Nctx, p.ClientID, corp, openID); err != nil {
		logger.Errorf("dingtalk uninstall db: %v", err)
	}
}
