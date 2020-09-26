package nux

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
)

type Loadavg struct {
	Avg1min  float64
	Avg5min  float64
	Avg15min float64
}

func (this *Loadavg) String() string {
	return fmt.Sprintf("<1min:%f, 5min:%f, 15min:%f>", this.Avg1min, this.Avg5min, this.Avg15min)
}

func LoadAvg() (*Loadavg, error) {

	loadAvg := Loadavg{}

	data, err := file.ToTrimString("/proc/loadavg")
	if err != nil {
		return nil, err
	}

	L := strings.Fields(data)
	if loadAvg.Avg1min, err = strconv.ParseFloat(L[0], 64); err != nil {
		return nil, err
	}
	if loadAvg.Avg5min, err = strconv.ParseFloat(L[1], 64); err != nil {
		return nil, err
	}
	if loadAvg.Avg15min, err = strconv.ParseFloat(L[2], 64); err != nil {
		return nil, err
	}

	return &loadAvg, nil
}
