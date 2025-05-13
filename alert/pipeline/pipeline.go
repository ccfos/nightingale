package pipeline

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

func Pipeline(ctx *ctx.Context, event *models.AlertCurEvent, processors []Processor) {
	for _, processor := range processors {
		processor.Process(ctx, event)
	}
}
