package nux

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
)

func SystemUptime() (days, hours, mins int64, err error) {
	var content string
	content, err = file.ToTrimString("/proc/uptime")
	if err != nil {
		return
	}

	fields := strings.Fields(content)
	if len(fields) < 2 {
		err = fmt.Errorf("/proc/uptime format not supported")
		return
	}

	secStr := fields[0]
	var secF float64
	secF, err = strconv.ParseFloat(secStr, 64)
	if err != nil {
		return
	}

	minTotal := secF / 60.0
	hourTotal := minTotal / 60.0

	days = int64(hourTotal / 24.0)
	hours = int64(hourTotal) - days*24
	mins = int64(minTotal) - (days * 60 * 24) - (hours * 60)

	return
}
