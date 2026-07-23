package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

const PlivoVoiceIdent = "plivo-voice"

type PlivoVoiceProvider struct{}

func (p *PlivoVoiceProvider) Ident() string {
	return PlivoVoiceIdent
}

func (p *PlivoVoiceProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != "plivo" {
		return errors.New("plivo voice provider requires request_type: plivo")
	}
	if err := config.ValidatePlivoRequestConfig(); err != nil {
		return err
	}
	if config.RequestConfig.PlivoRequestConfig.AnswerURL == "" {
		return errors.New("plivo voice provider requires answer_url")
	}
	return nil
}

func (p *PlivoVoiceProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req.Config.RequestConfig == nil || req.Config.RequestConfig.PlivoRequestConfig == nil {
		return &NotifyResult{Err: errors.New("plivo request config not found")}
	}
	if len(req.Sendtos) == 0 {
		return &NotifyResult{Err: errors.New("plivo voice requires at least one destination number in sendtos")}
	}

	cfg := req.Config.RequestConfig.PlivoRequestConfig
	if cfg.AnswerURL == "" {
		return &NotifyResult{Err: errors.New("plivo voice requires answer_url in the channel config")}
	}
	src := normalizePlivoNumber(cfg.SrcNumber)
	// Plivo defaults answer_method to POST; only override when the operator set one.
	answerMethod := strings.ToUpper(strings.TrimSpace(cfg.AnswerMethod))
	if answerMethod == "" {
		answerMethod = "POST"
	}
	endpoint := fmt.Sprintf("%s/%s/Call/", plivoAPIBase, cfg.AuthID)

	return notifyPlivo(ctx, req, endpoint, func(dst string) map[string]interface{} {
		return map[string]interface{}{
			"from":          src,
			"to":            dst,
			"answer_url":    cfg.AnswerURL,
			"answer_method": answerMethod,
		}
	})
}
