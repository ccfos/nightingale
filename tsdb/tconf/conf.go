package tconf

import (
	"fmt"
	"time"

	"github.com/alecthomas/units"
	"github.com/prometheus/common/model"
)

// EmbeddedTSDB is the config of the built-in time series database. When
// enabled, samples forwarded by pushgw are stored locally and a prometheus
// datasource pointing at the local query endpoints is auto-registered, so
// users can see metrics without deploying an external TSDB.
type EmbeddedTSDB struct {
	Enable               bool
	Dir                  string
	RetentionDuration    string
	MaxBytes             string
	OutOfOrderTimeWindow string
	QueryTimeout         string
	QueryMaxSamples      int
	QueryMaxConcurrency  int
	LookbackDelta        string
	// BasicAuthUser/Pass protect the /prometheus/api/v1/* endpoints. When
	// empty, those endpoints only accept requests from the n9e host itself
	// (see tsdb/router.Router.localOnly); setting them allows authenticated
	// remote access.
	BasicAuthUser string
	BasicAuthPass string
	// EnableAdminAPI controls whether the destructive admin endpoints
	// (delete_series / clean_tombstones) are registered. Off by default,
	// same as prometheus --web.enable-admin-api.
	EnableAdminAPI bool
	// DatasourceUrl overrides the url written into the auto-registered
	// datasource. Set it to a stable address (vip/domain) when the default
	// (127.0.0.1, or the detected ip once basic auth is configured) is not
	// what queriers should use. Setting it also lifts the local-only
	// restriction of the /prometheus/api/v1/* endpoints.
	DatasourceUrl string

	// parsed values, filled by PreCheck
	RetentionDurationValue    time.Duration `toml:"-" json:"-"`
	MaxBytesValue             int64         `toml:"-" json:"-"`
	OutOfOrderTimeWindowValue time.Duration `toml:"-" json:"-"`
	QueryTimeoutValue         time.Duration `toml:"-" json:"-"`
	LookbackDeltaValue        time.Duration `toml:"-" json:"-"`
}

func (c *EmbeddedTSDB) PreCheck() error {
	if !c.Enable {
		return nil
	}

	if c.Dir == "" {
		c.Dir = "data/tsdb"
	}

	if c.RetentionDuration == "" {
		c.RetentionDuration = "15d"
	}

	if c.OutOfOrderTimeWindow == "" {
		c.OutOfOrderTimeWindow = "10m"
	}

	if c.QueryTimeout == "" {
		c.QueryTimeout = "1m"
	}

	if c.QueryMaxSamples <= 0 {
		c.QueryMaxSamples = 50000000
	}

	// same default as prometheus --query.max-concurrency
	if c.QueryMaxConcurrency <= 0 {
		c.QueryMaxConcurrency = 20
	}

	if c.LookbackDelta == "" {
		c.LookbackDelta = "5m"
	}

	var err error
	if c.RetentionDurationValue, err = parseDuration("RetentionDuration", c.RetentionDuration); err != nil {
		return err
	}

	if c.OutOfOrderTimeWindowValue, err = parseDuration("OutOfOrderTimeWindow", c.OutOfOrderTimeWindow); err != nil {
		return err
	}

	if c.QueryTimeoutValue, err = parseDuration("QueryTimeout", c.QueryTimeout); err != nil {
		return err
	}

	if c.LookbackDeltaValue, err = parseDuration("LookbackDelta", c.LookbackDelta); err != nil {
		return err
	}

	if c.MaxBytes != "" && c.MaxBytes != "0" {
		bytes, err := units.ParseBase2Bytes(c.MaxBytes)
		if err != nil {
			return fmt.Errorf("EmbeddedTSDB.MaxBytes invalid (use units like 512MiB, 10GiB): %v", err)
		}
		c.MaxBytesValue = int64(bytes)
	}

	if (c.BasicAuthUser == "") != (c.BasicAuthPass == "") {
		return fmt.Errorf("EmbeddedTSDB.BasicAuthUser and BasicAuthPass must be set together")
	}

	return nil
}

func parseDuration(name, val string) (time.Duration, error) {
	d, err := model.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("EmbeddedTSDB.%s invalid (use durations like 10m, 1h, 15d): %v", name, err)
	}
	return time.Duration(d), nil
}
