package tplx

import (
	"fmt"
	"strconv"
)

// ToFloat64 convert interface to float64
func ToFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, nil
		}

		// try int
		if i, err := strconv.ParseInt(v, 0, 64); err == nil {
			return float64(i), nil
		}

		// try bool
		b, err := strconv.ParseBool(v)
		if err == nil {
			if b {
				return 1, nil
			} else {
				return 0, nil
			}
		}

		if v == "Yes" || v == "yes" || v == "YES" || v == "Y" || v == "ON" || v == "on" || v == "On" || v == "ok" || v == "up" {
			return 1, nil
		}

		if v == "No" || v == "no" || v == "NO" || v == "N" || v == "OFF" || v == "off" || v == "Off" || v == "fail" || v == "err" || v == "down" {
			return 0, nil
		}

		return 0, fmt.Errorf("unparseable value %v", v)
	case float64:
		return v, nil
	case uint64:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case bool:
		if v {
			return 1, nil
		} else {
			return 0, nil
		}
	case int:
		return float64(v), nil
	case float32:
		return float64(v), nil
	default:
		return strconv.ParseFloat(fmt.Sprint(v), 64)
	}
}
