package statsd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"

	tdigest "github.com/didi/nightingale/src/toolkits/go-tdigest"
)

type histogramAggregator struct {
	AggregatorNames []string
	digest          *tdigest.TDigest
	max             float64
	min             float64
	sum             float64
	cnt             int
}

func (self *histogramAggregator) new(aggregatorNames []string) (aggregator, error) {
	if len(aggregatorNames) < 1 {
		return nil, BadAggregatorNameError
	}
	ni := self.newInstence(aggregatorNames)
	return &ni, nil
}

// histogram类型可以接受一个或多个(并包模式下) value, 没有statusCode字段
// 形如 10.1{"\u2318"}10.2{"\u2318"}20.8
func (self *histogramAggregator) collect(values []string, metric string, argLines string) error {
	if len(values) < 1 {
		return fmt.Errorf("bad values")
	}

	for i := range values {
		parsed, err := strconv.ParseFloat(values[i], 64)
		if nil != err {
			return err
		}

		self.sum += parsed
		self.cnt += 1
		if self.max < parsed {
			self.max = parsed
		}
		if self.min > parsed {
			self.min = parsed
		}
		err = self.digest.Add(parsed, 1)
		return err
	}
	return nil
}

func (self *histogramAggregator) dump(points []*Point, timestamp int64,
	tags map[string]string, metric, argLines string) ([]*Point, error) {
	for _, aggregatorName := range self.AggregatorNames {
		value := 0.0
		percentile := ""
		switch aggregatorName {
		case "p99":
			value = self.digest.Quantile(0.99)
		case "p95":
			value = self.digest.Quantile(0.95)
		case "p90":
			value = self.digest.Quantile(0.90)
		case "p75":
			value = self.digest.Quantile(0.75)
		case "p50":
			value = self.digest.Quantile(0.5)
		case "p25":
			value = self.digest.Quantile(0.25)
		case "p10":
			value = self.digest.Quantile(0.10)
		case "p5":
			value = self.digest.Quantile(0.05)
		case "p1":
			value = self.digest.Quantile(0.01)
		case "max":
			value = self.max
			percentile = "max"
		case "min":
			value = self.min
			percentile = "min"
		case "sum":
			value = self.sum
			percentile = "sum"
		case "cnt":
			value = float64(self.cnt)
			percentile = "cnt"
		case "avg":
			if self.cnt > 0 {
				value = self.sum / float64(self.cnt)
			}
			percentile = "avg"
		default:
			continue
		}

		// TODO: 为什么不支持负数的统计? 先保持现状吧, 否则可能会影响rpc的latency指标
		if value < 0 {
			value = 0
		}

		myTags := map[string]string{}
		for k, v := range tags {
			myTags[k] = v
		}
		if percentile == "" {
			myTags["percentile"] = aggregatorName[1:]
		} else {
			myTags["percentile"] = percentile
		}
		points = append(points, &Point{
			Name:      metric,
			Timestamp: timestamp,
			Tags:      myTags,
			Value:     value,
		})
	}
	return points, nil
}

// 该统计不提供聚合功能, 因此下面的函数 不对 max/min/sum/cnt做处理
func (self *histogramAggregator) summarize(nsmetric, argLines string, newAggrs map[string]aggregator) {
	return
}

// aggr_rpc结构体聚合时使用
func (self *histogramAggregator) merge(toMerge aggregator) (aggregator, error) {
	that, ok := toMerge.(*histogramAggregator)
	if !ok {
		return nil, BadSummarizeAggregatorError
	}
	self.digest.Merge(that.digest)
	return self, nil
}

func (self *histogramAggregator) toMap() (map[string]interface{}, error) {
	digest, err := self.digest.AsBytes()
	if nil != err {
		return nil, err
	}

	aggregatorNames := make([]interface{}, 0)
	for _, aggregatorName := range self.AggregatorNames {
		aggregatorNames = append(aggregatorNames, aggregatorName)
	}
	return map[string]interface{}{
		"__aggregator__":  "histogram",
		"aggregatorNames": aggregatorNames,
		"digest":          base64.StdEncoding.EncodeToString(digest),
	}, nil
}

func (self *histogramAggregator) fromMap(serialized map[string]interface{}) (aggregator, error) {
	b, err := base64.StdEncoding.DecodeString(serialized["digest"].(string))
	if nil != err {
		return nil, fmt.Errorf("failed to deserialize: %v", serialized)
	}

	digest, err := tdigest.FromBytes(bytes.NewReader(b))
	if nil != err {
		return nil, fmt.Errorf("failed to deserialize: %v", serialized)
	}

	aggregator := &histogramAggregator{AggregatorNames: make([]string, 0), digest: digest}
	aggregatorNames := (serialized["aggregatorNames"]).([]interface{})
	for _, aggregatorName := range aggregatorNames {
		aggregator.AggregatorNames = append(aggregator.AggregatorNames, aggregatorName.(string))
	}

	return aggregator, nil
}

// internal functions
func (self histogramAggregator) newInstence(aggregatorNames []string) histogramAggregator {
	return histogramAggregator{
		AggregatorNames: aggregatorNames,
		digest:          tdigest.New(100),
		max:             float64(0.0),
		min:             float64(0.0),
		sum:             float64(0.0),
		cnt:             int(0),
	}
}
