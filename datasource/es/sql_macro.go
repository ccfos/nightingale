package es

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	MacroTimeFilter        = "$__timeFilter"
	MacroTimeFilterMS      = "$__timeFilter_ms"
	MacroDatetimeFilter    = "$__datetimeFilter"
	DefaultDatetimeFormat  = "2006-01-02T15:04:05.000Z"
	FallbackDatetimeFormat = "2006-01-02 15:04:05"
)

var (
	timeFilterRegex     = regexp.MustCompile(`\$__timeFilter\(([^)]+)\)`)
	timeFilterMSRegex   = regexp.MustCompile(`\$__timeFilter_ms\(([^)]+)\)`)
	datetimeFilterRegex = regexp.MustCompile(`\$__datetimeFilter\(([^)]+)\)`)
)

// ExpandTimeMacros expands time macros in SQL:
//   - $__timeFilter(col) -> col >= from AND col < to (seconds)
//   - $__timeFilter_ms(col) -> col >= from*1000 AND col < to*1000 (milliseconds)
//   - $__datetimeFilter(col) -> col >= 'formatted_from' AND col < 'formatted_to'
func ExpandTimeMacros(sql string, from, to int64, timezone string, timeFormat string) (string, error) {
	if !strings.Contains(sql, "$__") {
		return sql, nil
	}

	if timeFormat == "" {
		timeFormat = DefaultDatetimeFormat
	}

	loc := time.UTC
	if timezone != "" && timezone != "UTC" {
		if parsedLoc, err := time.LoadLocation(timezone); err == nil {
			loc = parsedLoc
		}
	}

	var err error

	sql, err = expandTimeFilter(sql, from, to)
	if err != nil {
		return "", err
	}

	sql, err = expandTimeFilterMS(sql, from, to)
	if err != nil {
		return "", err
	}

	sql, err = expandDatetimeFilter(sql, from, to, loc, timeFormat)
	if err != nil {
		return "", err
	}

	return sql, nil
}

func expandTimeFilter(sql string, from, to int64) (string, error) {
	return timeFilterRegex.ReplaceAllStringFunc(sql, func(match string) string {
		submatches := timeFilterRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		field := strings.TrimSpace(submatches[1])
		return fmt.Sprintf("(%s >= %d AND %s < %d)", field, from, field, to)
	}), nil
}

func expandTimeFilterMS(sql string, from, to int64) (string, error) {
	return timeFilterMSRegex.ReplaceAllStringFunc(sql, func(match string) string {
		submatches := timeFilterMSRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		field := strings.TrimSpace(submatches[1])
		fromMS := from * 1000
		toMS := to * 1000
		return fmt.Sprintf("(%s >= %d AND %s < %d)", field, fromMS, field, toMS)
	}), nil
}

func expandDatetimeFilter(sql string, from, to int64, loc *time.Location, timeFormat string) (string, error) {
	return datetimeFilterRegex.ReplaceAllStringFunc(sql, func(match string) string {
		submatches := datetimeFilterRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		field := strings.TrimSpace(submatches[1])
		fromTime := time.Unix(from, 0).In(loc)
		toTime := time.Unix(to, 0).In(loc)
		fromStr := fromTime.Format(timeFormat)
		toStr := toTime.Format(timeFormat)
		return fmt.Sprintf("(%s >= '%s' AND %s < '%s')", field, fromStr, field, toStr)
	}), nil
}
