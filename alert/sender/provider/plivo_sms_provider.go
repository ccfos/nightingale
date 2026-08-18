package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/ccfos/nightingale/v6/models"
)

const PlivoSmsIdent = "plivo-sms"

type PlivoSmsProvider struct{}

func (p *PlivoSmsProvider) Ident() string {
	return PlivoSmsIdent
}

func (p *PlivoSmsProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != "plivo" {
		return errors.New("plivo sms provider requires request_type: plivo")
	}
	return config.ValidatePlivoRequestConfig()
}

func (p *PlivoSmsProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req.Config.RequestConfig == nil || req.Config.RequestConfig.PlivoRequestConfig == nil {
		return &NotifyResult{Err: errors.New("plivo request config not found")}
	}
	if len(req.Sendtos) == 0 {
		return &NotifyResult{Err: errors.New("plivo sms requires at least one destination number in sendtos")}
	}

	cfg := req.Config.RequestConfig.PlivoRequestConfig
	text := plivoContent(req.TplContent)
	src := normalizePlivoNumber(cfg.SrcNumber)
	endpoint := fmt.Sprintf("%s/%s/Message/", plivoAPIBase, cfg.AuthID)

	return notifyPlivo(ctx, req, endpoint, func(dst string) map[string]interface{} {
		return map[string]interface{}{
			"src":  src,
			"dst":  dst,
			"text": text,
		}
	})
}
