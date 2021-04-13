package statsd

import (
	"fmt"
	"strconv"
	"strings"
)

type ratioAggregator struct {
	Counters map[string]float64
}

func (self *ratioAggregator) new(aggregatorNames []string) (aggregator, error) {
	if len(aggregatorNames) < 1 || aggregatorNames[0] != "r" {
		return nil, BadAggregatorNameError
	}
	return &ratioAggregator{Counters: map[string]float64{}}, nil
}

// ratio类型可以接受一个或多个(并包模式下) value, 有statusCode字段
// 旧版协议 形如: ok{"\u2318"}error{"\u2318"}ok
// 新版协议 形如: 1,ok{"\u2318"}1,error{"\u2318"}0,ok
func (self *ratioAggregator) collect(values []string, metric string, argLines string) error {
	if len(values) < 1 {
		return fmt.Errorf("bad values")
	}

	for i := range values {
		/*
			旧版协议: "error" 计数为 1, 形如"error,none", code取值为error(此处是values[0], none被截断)
			新版协议: "2,error" 计数为 2, 形如"2,error,none", code取值为error(此处是values[1], none被截断)
			为了兼容旧版
			1.只上报"error", 不包含","(逗号) 直接计数为1
			2.包含","(逗号), 且values[0]无法解析为数字, 计数为1, code取值values[0]
			3.包含","(逗号)且原来通过旧版协议上报了"2,error", 直接按新版处理, code从2变为error
		*/
		cvalues := strings.Split(values[i], CodeDelimiter)
		if len(cvalues) == 0 {
			continue
		}
		if len(cvalues) == 1 {
			code := values[0]
			self.Counters[code] += 1
			continue
		}

		code := cvalues[1]
		value, err := strconv.ParseFloat(cvalues[0], 64)
		if err != nil {
			value = float64(1) // 兼容旧版协议, 形如"error,something", 按照 1,error 处理
			code = values[0]
		}
		self.Counters[code] += value
	}

	return nil
}

func (self *ratioAggregator) dump(points []*Point, timestamp int64,
	tags map[string]string, metric, argLines string) ([]*Point, error) {
	return self._dump(false, points, timestamp, tags, metric, argLines)
}

func (self *ratioAggregator) summarize(nsmetric, argLines string, newAggrs map[string]aggregator) {
	return
}

func (self *ratioAggregator) merge(toMerge aggregator) (aggregator, error) {
	that := toMerge.(*ratioAggregator)
	for k, v2 := range that.Counters {
		_, found := self.Counters[k]
		if found {
			self.Counters[k] += v2
		} else {
			self.Counters[k] = v2
		}
	}
	return self, nil
}

func (self *ratioAggregator) toMap() (map[string]interface{}, error) {
	counters := map[string]float64{}
	for k, v := range self.Counters {
		counters[k] = v
	}

	return map[string]interface{}{
		"__aggregator__": "ratio",
		"counters":       counters,
	}, nil
}

func (self *ratioAggregator) fromMap(serialized map[string]interface{}) (aggregator, error) {
	aggr := &ratioAggregator{Counters: map[string]float64{}}

	counters := (serialized["counters"]).(map[string]interface{})
	for k, v := range counters {
		aggr.Counters[k] = v.(float64)
	}

	return aggr, nil
}

func (self *ratioAggregator) _dump(
	asTags bool, points []*Point, timestamp int64, tags map[string]string,
	metric string, argLines string) ([]*Point, error) {
	// 没有统计,则不dump
	if len(self.Counters) == 0 {
		return points, nil
	}

	convertedCounters := map[string]float64{}
	total := float64(0)
	for code, byCodeCount := range self.Counters {
		counter := byCodeCount
		convertedCounters[code] = counter
		total += counter
	}

	if total > 0 {
		for code := range self.Counters {
			myMetric := metric
			myTags := tags
			if asTags {
				myTags = map[string]string{}
				for tagk, tagv := range tags {
					myTags[tagk] = tagv
				}
				myTags["code"] = code
				myMetric = metric + ".ratio"
			} else {
				myMetric = metric + "." + code + ".ratio"
			}
			points = append(points, &Point{
				Name:      myMetric,
				Timestamp: timestamp,
				Tags:      myTags,
				Value:     convertedCounters[code] / total * 100,
			})
		}
	}

	points = append(points, &Point{
		Name:      metric + ".counter",
		Timestamp: timestamp,
		Tags:      tags,
		Value:     total,
	})
	return points, nil
}

////////////////////////////////////////////////////////////
// 			struct ratioAsTagsAggregator
////////////////////////////////////////////////////////////
type ratioAsTagsAggregator struct {
	ratioAggregator
}

func (self *ratioAsTagsAggregator) new(aggregatorNames []string) (aggregator, error) {
	if len(aggregatorNames) < 1 || aggregatorNames[0] != "rt" {
		return nil, BadAggregatorNameError
	}
	return &ratioAsTagsAggregator{ratioAggregator: ratioAggregator{Counters: map[string]float64{}}}, nil
}

func (self *ratioAsTagsAggregator) dump(points []*Point, timestamp int64,
	tags map[string]string, metric, argLines string) ([]*Point, error) {
	return self._dump(true, points, timestamp, tags, metric, argLines)
}

func (self *ratioAsTagsAggregator) merge(toMerge aggregator) (aggregator, error) {
	that := toMerge.(*ratioAsTagsAggregator)
	merged, err := self.ratioAggregator.merge(&that.ratioAggregator)
	if err != nil {
		return self, err
	}

	self.ratioAggregator = *(merged.(*ratioAggregator))
	return self, nil
}

func (self *ratioAsTagsAggregator) toMap() (map[string]interface{}, error) {
	counters := map[string]float64{}
	for k, v := range self.Counters {
		counters[k] = v
	}
	return map[string]interface{}{
		"__aggregator__": "ratioAsTags",
		"counters":       counters,
	}, nil
}

func (self *ratioAsTagsAggregator) fromMap(serialized map[string]interface{}) (aggregator, error) {
	aggr, err := self.ratioAggregator.fromMap(serialized)
	if err != nil {
		return nil, err
	}
	raggr := aggr.(*ratioAggregator)
	return &ratioAsTagsAggregator{ratioAggregator: *raggr}, nil
}
