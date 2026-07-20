package types

import "fmt"

// NormalizeUnixTimestamp converts a unix timestamp in seconds, milliseconds,
// microseconds, or nanoseconds to unix seconds.
func NormalizeUnixTimestamp(value float64) int64 {
	if value > 1e17 {
		return int64(value / 1e9)
	}
	if value > 1e14 {
		return int64(value / 1e6)
	}
	if value > 1e11 {
		return int64(value / 1000)
	}
	return int64(value)
}

// NormalizeUnixSeconds is a convenience wrapper for int64 unix timestamps.
func NormalizeUnixSeconds(value int64) int64 {
	if value <= 0 {
		return value
	}
	return NormalizeUnixTimestamp(float64(value))
}

// NormalizeUnixMillisecondsInt converts a unix timestamp in seconds, milliseconds,
// microseconds, or nanoseconds to unix milliseconds.
func NormalizeUnixMillisecondsInt(value int64) int64 {
	if value <= 0 {
		return value
	}
	v := float64(value)
	if v > 1e17 {
		return int64(v / 1e6)
	}
	if v > 1e14 {
		return int64(v / 1e3)
	}
	if v > 1e11 {
		return int64(v)
	}
	return int64(v * 1000)
}

// UnixRangeDurationSeconds returns the duration between two unix timestamps in
// seconds. Input bounds may be expressed in seconds, milliseconds, microseconds,
// or nanoseconds. Sub-second precision is truncated, which is sufficient for
// histogram step and LogQL range sizing.
func UnixRangeDurationSeconds(start, end int64) int64 {
	diff := NormalizeUnixSeconds(end) - NormalizeUnixSeconds(start)
	if diff <= 0 {
		return 1
	}
	return diff
}

// DefaultHistogramStepFromUnixRange returns a default histogram step for a time range
// whose bounds may be expressed in seconds, milliseconds, or other unix units.
func DefaultHistogramStepFromUnixRange(start, end int64) string {
	diffSec := UnixRangeDurationSeconds(start, end)
	return histogramWidthToStep(defaultHistogramWidthBySeconds(0, diffSec))
}

func defaultHistogramWidthBySeconds(start, end int64) int64 {
	diff := end - start
	switch {
	case diff <= 60:
		return 1
	case diff <= 300:
		return 5
	case diff <= 900:
		return 30
	case diff <= 1800:
		return 30
	case diff <= 3600:
		return 60
	case diff <= 3600*6:
		return 5 * 60
	case diff <= 3600*12:
		return 10 * 60
	case diff <= 3600*24:
		return 30 * 60
	case diff <= 3600*24*2:
		return 60 * 60
	case diff <= 3600*24*7:
		return 3 * 60 * 60
	case diff <= 3600*24*30:
		return 12 * 60 * 60
	case diff <= 3600*24*90:
		return 24 * 60 * 60
	default:
		return 2 * 24 * 60 * 60
	}
}

func histogramWidthToStep(width int64) string {
	switch {
	case width%86400 == 0:
		return fmt.Sprintf("%dd", width/86400)
	case width%3600 == 0:
		return fmt.Sprintf("%dh", width/3600)
	case width%60 == 0:
		return fmt.Sprintf("%dm", width/60)
	default:
		return fmt.Sprintf("%ds", width)
	}
}
