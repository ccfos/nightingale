package provider

import (
	"context"

	"github.com/ccfos/nightingale/v6/models"
)

type GenericHTTPProvider struct{}

func (p *GenericHTTPProvider) Ident() string { return "http" }

func (p *GenericHTTPProvider) Check(config *models.NotifyChannelConfig) error {
	return nil
}

func (p *GenericHTTPProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	return nil
}

func (p *GenericHTTPProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return nil
}
