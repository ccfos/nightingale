package types

import "fmt"

// DefaultHistogramStep returns a default histogram step for the given unix second time range.
func DefaultHistogramStep(start, end int64) string {
	return histogramWidthToStep(defaultHistogramWidthBySeconds(start, end))
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
