package tplx

import (
	"fmt"
	"html/template"
	"math"
	"regexp"
	"strings"
	"time"
)

var TemplateFuncMap = template.FuncMap{
	"unescaped":  func(str string) interface{} { return template.HTML(str) },
	"urlconvert": func(str string) interface{} { return template.URL(str) },
	"timeformat": func(ts int64, pattern ...string) string {
		defp := "2006-01-02 15:04:05"
		if len(pattern) > 0 {
			defp = pattern[0]
		}
		return time.Unix(ts, 0).Format(defp)
	},
	"timestamp": func(pattern ...string) string {
		defp := "2006-01-02 15:04:05"
		if len(pattern) > 0 {
			defp = pattern[0]
		}
		return time.Now().Format(defp)
	},
	"args": func(args ...interface{}) map[string]interface{} {
		result := make(map[string]interface{})
		for i, a := range args {
			result[fmt.Sprintf("arg%d", i)] = a
		}
		return result
	},
	"reReplaceAll": func(pattern, repl, text string) string {
		re := regexp.MustCompile(pattern)
		return re.ReplaceAllString(text, repl)
	},
	"match":   regexp.MatchString,
	"toUpper": strings.ToUpper,
	"toLower": strings.ToLower,
	"humanize": func(v float64) string {
		if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Sprintf("%.2f", v)
		}
		if math.Abs(v) >= 1 {
			prefix := ""
			for _, p := range []string{"k", "M", "G", "T", "P", "E", "Z", "Y"} {
				if math.Abs(v) < 1000 {
					break
				}
				prefix = p
				v /= 1000
			}
			return fmt.Sprintf("%.2f%s", v, prefix)
		}
		prefix := ""
		for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
			if math.Abs(v) >= 1 {
				break
			}
			prefix = p
			v *= 1000
		}
		return fmt.Sprintf("%.2f%s", v, prefix)
	},
	"humanize1024": func(v float64) string {
		if math.Abs(v) <= 1 || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Sprintf("%.4g", v)
		}
		prefix := ""
		for _, p := range []string{"ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi", "Yi"} {
			if math.Abs(v) < 1024 {
				break
			}
			prefix = p
			v /= 1024
		}
		return fmt.Sprintf("%.4g%s", v, prefix)
	},
	"humanizeDuration": func(v float64) string {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Sprintf("%.4g", v)
		}
		if v == 0 {
			return fmt.Sprintf("%.4gs", v)
		}
		if math.Abs(v) >= 1 {
			sign := ""
			if v < 0 {
				sign = "-"
				v = -v
			}
			seconds := int64(v) % 60
			minutes := (int64(v) / 60) % 60
			hours := (int64(v) / 60 / 60) % 24
			days := int64(v) / 60 / 60 / 24
			// For days to minutes, we display seconds as an integer.
			if days != 0 {
				return fmt.Sprintf("%s%dd %dh %dm %ds", sign, days, hours, minutes, seconds)
			}
			if hours != 0 {
				return fmt.Sprintf("%s%dh %dm %ds", sign, hours, minutes, seconds)
			}
			if minutes != 0 {
				return fmt.Sprintf("%s%dm %ds", sign, minutes, seconds)
			}
			// For seconds, we display 4 significant digits.
			return fmt.Sprintf("%s%.4gs", sign, v)
		}
		prefix := ""
		for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
			if math.Abs(v) >= 1 {
				break
			}
			prefix = p
			v *= 1000
		}
		return fmt.Sprintf("%.4g%ss", v, prefix)
	},
	"humanizePercentage": func(v float64) string {
		return fmt.Sprintf("%.2f%%", v*100)
	},
	"humanizePercentageH": func(v float64) string {
		return fmt.Sprintf("%.2f%%", v)
	},
}
