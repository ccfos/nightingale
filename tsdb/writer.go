package tsdb

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"
)

// AppendTimeSeries appends remote-write series into the local storage, used
// by the /prometheus/api/v1/write endpoint. Per-sample append errors (mostly
// out-of-order/duplicate samples) are counted and skipped so one bad agent
// cannot fail the whole batch; only a commit failure is returned.
func (in *Instance) AppendTimeSeries(items []prompb.TimeSeries) error {
	if len(items) == 0 {
		return nil
	}

	app := in.DB.Appender(context.Background())

	var errCount int
	var lastErr error
	var builder labels.ScratchBuilder

	for i := range items {
		builder.Reset()
		for _, l := range items[i].Labels {
			builder.Add(l.Name, l.Value)
		}
		builder.Sort()
		lset := builder.Labels()

		for _, s := range items[i].Samples {
			if _, err := app.Append(0, lset, s.Timestamp, s.Value); err != nil {
				errCount++
				lastErr = err
			}
		}
	}

	if err := app.Commit(); err != nil {
		return err
	}

	if errCount > 0 {
		logger.Warningf("embedded tsdb append fail, dropped samples: %d, last error: %v", errCount, lastErr)
	}

	return nil
}
