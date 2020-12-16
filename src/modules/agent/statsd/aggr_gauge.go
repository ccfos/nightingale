package statsd

import (
	"fmt"
	"strconv"
)

type gaugeAggregator struct {
	Gauge float64
}

func (self *gaugeAggregator) new(aggregatorNames []string) (aggregator, error) {
	if len(aggregatorNames) < 1 || aggregatorNames[0] != "g" {
		return nil, BadAggregatorNameError
	}
	return &gaugeAggregator{}, nil
}

// gauge类型可以接受一个或多个(并包模式下) value, 没有statusCode字段, 不在sdk做并包
// 形如 10{"\u2318"}1{"\u2318"}20
func (self *gaugeAggregator) collect(values []string, metric string, argLines string) error {
	if len(values) < 1 {
		return fmt.Errorf("bad values")
	}

	for i := range values {
		delta := float64(0.0)
		parsed, err := strconv.ParseFloat(values[i], 64)
		if err != nil {
			return err
		}
		delta = parsed
		self.Gauge = delta
	}

	return nil
}

func (self *gaugeAggregator) dump(points []*Point, timestamp int64,
	tags map[string]string, metric, argLines string) ([]*Point, error) {

	points = append(points, &Point{
		Name:      metric + ".gauge",
		Timestamp: timestamp,
		Tags:      tags,
		Value:     self.Gauge,
	})
	return points, nil
}

// 不支持聚合功能
func (self *gaugeAggregator) summarize(nsmetric, argLines string, newAggrs map[string]aggregator) {
	return
}

func (self *gaugeAggregator) merge(toMerge aggregator) (aggregator, error) {
	return self, nil
}

func (self *gaugeAggregator) toMap() (map[string]interface{}, error) {
	return map[string]interface{}{
		"__aggregator__": "gauge",
		"gauge":          self.Gauge,
	}, nil
}

func (self gaugeAggregator) fromMap(serialized map[string]interface{}) (aggregator, error) {
	return &gaugeAggregator{Gauge: serialized["gauge"].(float64)}, nil
}
