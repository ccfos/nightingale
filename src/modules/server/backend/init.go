package backend

import (
	"context"
	"fmt"
	"time"

	"github.com/didi/nightingale/v4/src/modules/server/backend/influxdb"
	"github.com/didi/nightingale/v4/src/modules/server/backend/m3db"
	"github.com/didi/nightingale/v4/src/modules/server/backend/prom"
	"github.com/didi/nightingale/v4/src/modules/server/backend/tsdb"
)

type BackendSection struct {
	DataSource string `yaml:"datasource"`
	StraPath   string `yaml:"straPath"`

	M3db     m3db.M3dbSection         `yaml:"m3db"`
	Prom     prom.PromSection         `yaml:"prom"`
	Tsdb     tsdb.TsdbSection         `yaml:"tsdb"`
	Influxdb influxdb.InfluxdbSection `yaml:"influxdb"`
	OpenTsdb OpenTsdbSection          `yaml:"opentsdb"`
	Kafka    KafkaSection             `yaml:"kafka"`
}

var (
	defaultDataSource    string
	StraPath             string
	tsdbDataSource       *tsdb.TsdbDataSource
	openTSDBPushEndpoint *OpenTsdbPushEndpoint
	influxdbDataSource   *influxdb.InfluxdbDataSource
	kafkaPushEndpoint    *KafkaPushEndpoint
	m3dbDataSource       *m3db.Client
	promDataSource       *prom.PromDataSource
)

func Init(cfg BackendSection) error {
	defaultDataSource = cfg.DataSource
	StraPath = cfg.StraPath

	// init tsdb
	if cfg.Tsdb.Enabled {
		tsdbDataSource = &tsdb.TsdbDataSource{
			Section:               cfg.Tsdb,
			SendQueueMaxSize:      DefaultSendQueueMaxSize,
			SendTaskSleepInterval: DefaultSendTaskSleepInterval,
		}
		tsdbDataSource.Init() // register
		RegisterDataSource(tsdbDataSource.Section.Name, tsdbDataSource)
	}

	// init influxdb
	if cfg.Influxdb.Enabled {
		influxdbDataSource = &influxdb.InfluxdbDataSource{
			Section:               cfg.Influxdb,
			SendQueueMaxSize:      DefaultSendQueueMaxSize,
			SendTaskSleepInterval: DefaultSendTaskSleepInterval,
		}
		influxdbDataSource.Init()
		// register
		RegisterDataSource(influxdbDataSource.Section.Name, influxdbDataSource)

	}
	// init opentsdb
	if cfg.OpenTsdb.Enabled {
		openTSDBPushEndpoint = &OpenTsdbPushEndpoint{
			Section: cfg.OpenTsdb,
		}
		openTSDBPushEndpoint.Init()
		// register
		RegisterPushEndpoint(openTSDBPushEndpoint.Section.Name, openTSDBPushEndpoint)
	}
	// init kafka
	if cfg.Kafka.Enabled {
		kafkaPushEndpoint = &KafkaPushEndpoint{
			Section: cfg.Kafka,
		}
		kafkaPushEndpoint.Init()
		// register
		RegisterPushEndpoint(kafkaPushEndpoint.Section.Name, kafkaPushEndpoint)
	}
	// init m3db
	if cfg.M3db.Enabled {
		var err error
		d := time.Now().Add(time.Second * time.Duration(cfg.M3db.Timeout))
		ctx, cancel := context.WithDeadline(context.Background(), d)

		go func() {
			m3dbDataSource, err = m3db.NewClient(cfg.M3db)
			if err != nil {
				err = fmt.Errorf("unable to new m3db client: %v", err)
			}
			RegisterDataSource(cfg.M3db.Name, m3dbDataSource)
			cancel()
		}()

		<-ctx.Done()
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil && err != context.Canceled {
			return fmt.Errorf("new m3db client err: %s", err)
		}
	}

	// init VictoriaMetrics
	if cfg.Prom.Enabled {
		promDataSource = &prom.PromDataSource{
			Section:               cfg.Prom,
			SendQueueMaxSize:      DefaultSendQueueMaxSize,
			SendTaskSleepInterval: DefaultSendTaskSleepInterval,
		}
		promDataSource.Init()
		RegisterDataSource(promDataSource.Section.Name, promDataSource)
	}

	return nil
}
