package memsto

import "github.com/prometheus/client_golang/prometheus"

type Stats struct {
	GaugeCronDuration *prometheus.GaugeVec
	GaugeSyncNumber   *prometheus.GaugeVec
}

func NewSyncStats() *Stats {
	GaugeCronDuration := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "n9e",
		Subsystem: "cron",
		Name:      "duration",
		Help:      "Cron method use duration, unit: ms.",
	}, []string{"name"})

	GaugeSyncNumber := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "n9e",
		Subsystem: "cron",
		Name:      "sync_number",
		Help:      "Cron sync number.",
	}, []string{"name"})

	prometheus.MustRegister(
		GaugeCronDuration,
		GaugeSyncNumber,
	)

	return &Stats{
		GaugeCronDuration: GaugeCronDuration,
		GaugeSyncNumber:   GaugeSyncNumber,
	}
}
