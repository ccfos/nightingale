package backend

import (
	"log"

	"github.com/didi/nightingale/src/modules/transfer/backend/influxdb"
	"github.com/didi/nightingale/src/modules/transfer/backend/m3db"
	"github.com/didi/nightingale/src/modules/transfer/backend/tsdb"
)

type BackendSection struct {
	DataSource string `yaml:"datasource"`
	StraPath   string `yaml:"straPath"`

	Judge    JudgeSection             `yaml:"judge"`
	M3db     m3db.M3dbSection         `yaml:"m3db"`
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
)

func Init(cfg BackendSection) {
	defaultDataSource = cfg.DataSource
	StraPath = cfg.StraPath

	// init judge
	InitJudge(cfg.Judge)

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
		m3dbDataSource, err = m3db.NewClient(cfg.M3db.Namespace, &cfg.M3db.Config)
		if err != nil {
			log.Fatalf("unable to new m3db client: %v", err)
		}
		RegisterDataSource(cfg.M3db.Name, m3dbDataSource)
	}
}
