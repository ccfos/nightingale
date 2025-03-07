// @Author: Ciusyan 5/20/24

package sqlbase

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/types"

	"github.com/prometheus/common/model"
	"gorm.io/gorm"
)

type QueryParam struct {
	Sql  string     `json:"sql"`
	Keys types.Keys `json:"keys" mapstructure:"keys"`
}

var (
	BannedOp = map[string]struct{}{
		"CREATE":   {},
		"INSERT":   {},
		"UPDATE":   {},
		"DELETE":   {},
		"ALTER":    {},
		"REVOKE":   {},
		"DROP":     {},
		"RENAME":   {},
		"TRUNCATE": {},
		"SET":      {},
	}
)

// Query executes a given SQL query and returns the results
func Query(ctx context.Context, db *gorm.DB, query *QueryParam) ([]map[string]interface{}, error) {
	// Validate SQL to prevent write operations if needed
	sqlItem := strings.Split(strings.ToUpper(query.Sql), " ")
	for _, item := range sqlItem {
		if _, ok := BannedOp[item]; ok {
			return nil, fmt.Errorf("operation %s is forbidden, only read operations are allowed, please check your SQL", item)
		}
	}

	return ExecQuery(ctx, db, query.Sql)
}

// QueryTimeseries executes a time series data query using the given parameters
func QueryTimeseries(ctx context.Context, db *gorm.DB, query *QueryParam, ignoreDefault ...bool) ([]types.MetricValues, error) {
	rows, err := Query(ctx, db, query)
	if err != nil {
		return nil, err
	}

	return FormatMetricValues(query.Keys, rows, ignoreDefault...), nil
}

func FormatMetricValues(keys types.Keys, rows []map[string]interface{}, ignoreDefault ...bool) []types.MetricValues {
	ignore := false
	if len(ignoreDefault) > 0 {
		ignore = ignoreDefault[0]
	}

	keyMap := make(map[string]string)
	for _, valueMetric := range strings.Split(keys.ValueKey, " ") {
		keyMap[valueMetric] = "value"
	}

	for _, labelMetric := range strings.Split(keys.LabelKey, " ") {
		keyMap[labelMetric] = "label"
	}

	if keys.TimeKey == "" {
		keys.TimeKey = "time"
	}

	if len(keys.TimeKey) > 0 {
		keyMap[keys.TimeKey] = "time"
	}

	var dataResps []types.MetricValues
	dataMap := make(map[string]*types.MetricValues)

	for _, row := range rows {
		labels := make(map[string]string)
		metricValue := make(map[string]float64)
		metricTs := make(map[string]float64)

		// Process each column based on its designated role (value, label, time)
		for k, v := range row {
			switch keyMap[k] {
			case "value":
				val, err := ParseFloat64Value(v)
				if err != nil {
					continue
				}
				metricValue[k] = val
			case "label":
				labels[k] = fmt.Sprintf("%v", v)
			case "time":
				ts, err := ParseTime(v, keys.TimeFormat)
				if err != nil {
					continue
				}
				metricTs[k] = float64(ts.Unix())
			default:
				// Default to labels for any unrecognized columns
				if !ignore {
					labels[k] = fmt.Sprintf("%v", v)
				}
			}
		}

		// Compile and store the metric values
		for metricName, value := range metricValue {
			metrics := make(model.Metric)
			var labelsStr []string

			for k1, v1 := range labels {
				metrics[model.LabelName(k1)] = model.LabelValue(v1)
				labelsStr = append(labelsStr, fmt.Sprintf("%s=%s", k1, v1))
			}
			metrics["__name__"] = model.LabelValue(metricName)
			labelsStr = append(labelsStr, fmt.Sprintf("__name__=%s", metricName))

			// Hash the labels to use as a key
			sort.Strings(labelsStr)
			labelsStrHash := fmt.Sprintf("%x", md5.Sum([]byte(strings.Join(labelsStr, ","))))

			// Append new values to the existing metric, if present
			ts, exists := metricTs[keys.TimeKey]
			if !exists {
				ts = float64(time.Now().Unix()) // Default to current time if not specified
			}

			valuePair := []float64{ts, value}
			if existing, ok := dataMap[labelsStrHash]; ok {
				existing.Values = append(existing.Values, valuePair)
			} else {
				dataResp := types.MetricValues{
					Metric: metrics,
					Values: [][]float64{valuePair},
				}
				dataMap[labelsStrHash] = &dataResp
			}
		}
	}

	// Convert the map to a slice for the response
	for _, v := range dataMap {
		sort.Slice(v.Values, func(i, j int) bool { return v.Values[i][0] < v.Values[j][0] }) // Sort by timestamp
		dataResps = append(dataResps, *v)
	}

	return dataResps
}

// ParseFloat64Value attempts to convert an interface{} to float64 using reflection
func ParseFloat64Value(val interface{}) (float64, error) {
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Float64, reflect.Float32:
		return v.Float(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), nil
	case reflect.String:
		return strconv.ParseFloat(v.String(), 64)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return strconv.ParseFloat(string(v.Bytes()), 64)
		}
	case reflect.Interface:
		return ParseFloat64Value(v.Interface())
	case reflect.Ptr:
		if !v.IsNil() {
			return ParseFloat64Value(v.Elem().Interface())
		}
	case reflect.Struct:
		if num, ok := val.(json.Number); ok {
			return num.Float64()
		}
	}
	return 0, fmt.Errorf("cannot convert type %T to float64", val)
}

// ParseTime attempts to parse a time value from an interface{} using a specified format
func ParseTime(val interface{}, format string) (time.Time, error) {
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.String:
		str := v.String()
		return parseTimeFromString(str, format)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			str := string(v.Bytes())
			return parseTimeFromString(str, format)
		}
	case reflect.Int, reflect.Int64:
		return time.Unix(v.Int(), 0), nil
	case reflect.Float64:
		return time.Unix(int64(v.Float()), 0), nil
	case reflect.Interface:
		return ParseTime(v.Interface(), format)
	case reflect.Ptr:
		if !v.IsNil() {
			return ParseTime(v.Elem().Interface(), format)
		}
	case reflect.Struct:
		if t, ok := val.(time.Time); ok {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time value type: %v", val)
}

func parseTimeFromString(str, format string) (time.Time, error) {
	// If a custom time format is provided, use it to parse the string
	if format != "" {
		parsedTime, err := time.Parse(format, str)
		if err == nil {
			return parsedTime, nil
		}
		return time.Time{}, fmt.Errorf("failed to parse time '%s' with format '%s': %v", str, format, err)
	}

	// Try to parse the string as RFC3339, RFC3339Nano, or Unix timestamp
	if parsedTime, err := time.Parse(time.RFC3339, str); err == nil {
		return parsedTime, nil
	}
	if parsedTime, err := time.Parse(time.RFC3339Nano, str); err == nil {
		return parsedTime, nil
	}
	if timestamp, err := strconv.ParseInt(str, 10, 64); err == nil {
		return time.Unix(timestamp, 0), nil
	}
	if timestamp, err := strconv.ParseFloat(str, 64); err == nil {
		return time.Unix(int64(timestamp), 0), nil
	}

	return time.Time{}, fmt.Errorf("failed to parse time '%s'", str)
}
