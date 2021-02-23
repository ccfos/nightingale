package demo

import (
	"math"
	"strconv"
	"time"

	"github.com/influxdata/telegraf"
)

type Demo struct {
	Period int `toml:"period"`
	Count  int `toml:"count"`

	initDone bool
	cos      *cos
}

func (d *Demo) SampleConfig() string {
	return `
  ## The period of the function, in seconds
  period = 600
  ## The Count of the series
  count = 3
`
}

func (d *Demo) Description() string {
	return "telegraf demo plugin"
}

func (d *Demo) Init() {
	d.cos = &cos{
		period: float64(d.Period),
		offset: (d.Period / d.Count),
	}

	d.initDone = true
}

func (d *Demo) Gather(acc telegraf.Accumulator) error {
	if !d.initDone {
		d.Init()
	}

	fields := make(map[string]interface{})
	tags := map[string]string{}
	for i := 0; i < d.Count; i++ {
		tags["n"] = strconv.Itoa(i)
		fields["value"] = d.cos.value(i)
		acc.AddFields("demo", fields, tags)
	}
	return nil
}

type cos struct {
	period float64
	offset int
}

func (c *cos) value(i int) float64 {
	return math.Cos(2 * math.Pi * (float64(time.Now().Unix()+int64(c.offset*i)) / c.period))
}
