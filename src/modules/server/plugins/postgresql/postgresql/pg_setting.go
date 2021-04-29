package postgresql

import (
	"fmt"
	"github.com/influxdata/telegraf"
	"math"
	"strconv"
	"strings"
)

// Query the pg_settings view containing runtime variables
func querySettings(host string, server *Server, acc telegraf.Accumulator) error {
	// pg_settings docs: https://www.postgresql.org/docs/current/static/view-pg-settings.html
	//
	// NOTE: If you add more vartypes here, you must update the supported
	// types in normaliseUnit() below
	query := "SELECT name, setting, COALESCE(unit, ''), short_desc, vartype FROM pg_settings WHERE vartype IN ('bool', 'integer', 'real');"
	rows, err := server.db.Query(query)
	if err != nil {
		return fmt.Errorf("Error running query on database %q: %s %v", server, "pg", err)
	}
	defer rows.Close() // nolint: errcheck
	var fields =make(map[string]interface{})
	for rows.Next() {
		s := &pgSetting{}
		err = rows.Scan(&s.name, &s.setting, &s.unit, &s.shortDesc, &s.vartype)
		if err != nil {
			return fmt.Errorf("Error retrieving rows on %q: %s %v", server, "pg", err)
		}
		// 处理结果
		k, v := s.metric()
		fields[k] = v
	}
	acc.AddGauge("postgresql_pg_settings",fields, map[string]string{"server": host})
	return nil
}

// pgSetting is represents a PostgreSQL runtime variable as returned by the
// pg_settings view.
type pgSetting struct {
	name, setting, unit, shortDesc, vartype string
}

func (s *pgSetting) metric() (name string, val float64) {
	var (
		err  error
		unit = s.unit // nolint: ineffassign
	)
	name = strings.Replace(s.name, ".", "_", -1)
	switch s.vartype {
	case "bool":
		if s.setting == "on" {
			val = 1
		}
	case "integer", "real":
		if val, unit, err = s.normaliseUnit(); err != nil {
			// Panic, since we should recognise all units
			// and don't want to silently exlude metrics
			panic(err)
		}

		if len(unit) > 0 {
			name = fmt.Sprintf("%s_%s", name, unit)
		}
	default:
		// Panic because we got a type we didn't ask for
		panic(fmt.Sprintf("Unsupported vartype %q", s.vartype))
	}
	return name, val
	//acc.AddGauge("postgresql_pg_settings", map[string]interface{}{name: val}, map[string]string{"server": host})
}

// TODO: fix linter override
// nolint: nakedret
func (s *pgSetting) normaliseUnit() (val float64, unit string, err error) {
	val, err = strconv.ParseFloat(s.setting, 64)
	if err != nil {
		return val, unit, fmt.Errorf("Error converting setting %q value %q to float: %s", s.name, s.setting, err)
	}

	// Units defined in: https://www.postgresql.org/docs/current/static/config-setting.html
	switch s.unit {
	case "":
		return
	case "ms", "s", "min", "h", "d":
		unit = "seconds"
	case "B", "kB", "MB", "GB", "TB", "8kB", "16kB", "32kB", "16MB", "32MB", "64MB":
		unit = "bytes"
	default:
		err = fmt.Errorf("Unknown unit for runtime variable: %q", s.unit)
		return
	}

	// -1 is special, don't modify the value
	if val == -1 {
		return
	}

	switch s.unit {
	case "ms":
		val /= 1000
	case "min":
		val *= 60
	case "h":
		val *= 60 * 60
	case "d":
		val *= 60 * 60 * 24
	case "kB":
		val *= math.Pow(2, 10)
	case "MB":
		val *= math.Pow(2, 20)
	case "GB":
		val *= math.Pow(2, 30)
	case "TB":
		val *= math.Pow(2, 40)
	case "8kB":
		val *= math.Pow(2, 13)
	case "16kB":
		val *= math.Pow(2, 14)
	case "32kB":
		val *= math.Pow(2, 15)
	case "16MB":
		val *= math.Pow(2, 24)
	case "32MB":
		val *= math.Pow(2, 25)
	case "64MB":
		val *= math.Pow(2, 26)
	}

	return
}
