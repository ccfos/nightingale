package statsd

import (
	"fmt"
	"sort"
	"strconv"
)

// maxAggregator

// counter enhance, aggr="ce"
type counterEAggregator struct {
	Counter       float64
	Stats         map[int64]float64 // 不需要加锁, 单线程
	lastTimestamp int64
	delta         float64
	raw           bool // 原始统计(true) or 聚合后的统计(false), bool型初始化是false
}

func (self *counterEAggregator) new(aggregatorNames []string) (aggregator, error) {
	if len(aggregatorNames) < 1 || aggregatorNames[0] != "ce" {
		return nil, BadAggregatorNameError
	}
	return &counterEAggregator{
		Stats:         make(map[int64]float64),
		lastTimestamp: GetTimestamp(),
		delta:         0,
		raw:           true,
	}, nil
}

// counterE类型可以接受一个或多个(并包模式下) value, 没有statusCode字段, 不在sdk做并包
// 形如 10{"\u2318"}1{"\u2318"}20
func (self *counterEAggregator) collect(values []string, metric string, argLines string) error {
	if len(values) < 1 {
		return fmt.Errorf("bad values")
	}

	ts := GetTimestamp()

	for i := range values {
		delta := float64(0.0)
		parsed, err := strconv.ParseFloat(values[i], 64)
		if nil != err {
			return err
		}

		delta = parsed
		self.Counter += delta

		if ts > self.lastTimestamp {
			self.Stats[self.lastTimestamp] = self.delta
			self.delta = delta
			self.lastTimestamp = ts
		} else {
			self.delta += delta
		}

	}

	return nil
}

func (self *counterEAggregator) dump(points []*Point, timestamp int64,
	tags map[string]string, metric, argLines string) ([]*Point, error) {

	points = append(points, &Point{
		Name:      metric + ".counter",
		Timestamp: timestamp,
		Tags:      tags,
		Value:     self.Counter,
	})

	// 原始统计出max/min值,聚合的结果不出
	if self.raw {
		max := float64(0.0)
		min := float64(0.0)
		sum := float64(0.0)
		cnt := len(self.Stats)
		if cnt > 0 {
			flag := true
			for _, value := range self.Stats {
				sum += value
				if flag {
					max = value
					min = value
					flag = false
					continue
				}

				if value > max {
					max = value
				}
				if value < min {
					min = value
				}
			}
		} else {
			cnt = 1
		}
		points = append(points, &Point{
			Name:      metric + ".counter.max",
			Timestamp: timestamp,
			Tags:      tags,
			Value:     max,
		})
		points = append(points, &Point{
			Name:      metric + ".counter.min",
			Timestamp: timestamp,
			Tags:      tags,
			Value:     min,
		})
		points = append(points, &Point{
			Name:      metric + ".counter.avg",
			Timestamp: timestamp,
			Tags:      tags,
			Value:     sum / float64(cnt),
		})
	}

	return points, nil
}

func (self *counterEAggregator) summarize(nsmetric, argLines string, newAggrs map[string]aggregator) {
	// 准备: ns/metric
	//items, _ := Func{}.TranslateMetricLine(nsmetric)
	//ns := items[0]
	//metric := items[1]

	// 黑名单

	// 准备: tags
	tags, _, err := Func{}.TranslateArgLines(argLines)
	if err != nil {
		return
	}

	// 未统计的delta补齐到stats中
	if self.raw && self.delta > 0 {
		self.Stats[self.lastTimestamp] = self.delta
	}

	// 只做默认聚合
	self.doAggr(tags, newAggrs)
	// 本机聚合

	return
}

func (self *counterEAggregator) merge(toMerge aggregator) (aggregator, error) {
	that := toMerge.(*counterEAggregator)
	self.Counter += that.Counter

	for ts, value := range that.Stats {
		if _, found := self.Stats[ts]; found {
			self.Stats[ts] += value
		} else {
			self.Stats[ts] = value
		}
	}
	return self, nil
}

func (self *counterEAggregator) toMap() (map[string]interface{}, error) {
	stats := map[int64]interface{}{}
	for k, v := range self.Stats {
		stats[k] = v
	}

	return map[string]interface{}{
		"__aggregator__": "counterE",
		"counter":        self.Counter,
		"stats":          stats,
	}, nil
}

func (self counterEAggregator) fromMap(serialized map[string]interface{}) (aggregator, error) {
	// raw字段默认是false
	aggregator := &counterEAggregator{Counter: serialized["counter"].(float64), Stats: map[int64]float64{}}
	stats := (serialized["stats"]).(map[int64]interface{})
	for k, v := range stats {
		aggregator.Stats[k] = v.(float64)
	}
	return aggregator, nil
}

// internals
func (self counterEAggregator) addSummarizeAggregator(argLines string, toMerge *counterEAggregator, newAggrs map[string]aggregator) {
	aggr, ok := newAggrs[argLines]
	if !(ok && aggr != nil) {
		nAggr, err := toMerge.clone()
		if err == nil {
			newAggrs[argLines] = nAggr
		}
	} else {
		aggr.merge(toMerge)
	}
}

func (self *counterEAggregator) clone() (aggregator, error) {
	maps, err := self.toMap()
	if err != nil {
		return nil, err
	}

	aggr, err := counterEAggregator{}.fromMap(maps)
	if err != nil {
		return nil, err
	}

	return aggr, nil
}

func (self *counterEAggregator) doAggr(tags map[string]string, newAggrs map[string]aggregator, aggrTagksList ...[][]string) {
	tagks := make([]string, 0)
	for k, _ := range tags {
		tagks = append(tagks, k)
	}

	tagkNum := len(tagks)
	if tagkNum == 0 {
		return
	}
	sort.Strings(tagks)

	// get formator
	formator := ""
	for i := 0; i < tagkNum; i++ {
		formator += tagks[i] + "=%s\n"
	}
	formator += "ce"

	// 聚合所有维度
	ntagvs_all := make([]interface{}, tagkNum)
	for i := 0; i < tagkNum; i++ {
		ntagvs_all[i] = "<all>"
	}
	summarizedTags := fmt.Sprintf(formator, ntagvs_all...)
	counterEAggregator{}.addSummarizeAggregator(summarizedTags, self, newAggrs)

	// 聚合指定维度
	if len(aggrTagksList) > 0 {
		for i := 0; i < len(aggrTagksList[0]); i++ {
			aggrTagks := aggrTagksList[0][i]
			// 判断合法性
			if !(len(aggrTagks) > 0 && len(aggrTagks) < tagkNum && // ==tagsNum 会造成 所有维度 的重复聚合
				(Func{}).IsSubKeys(aggrTagks, tags)) { // 监控数据 有 指定的聚合维度
				continue
			}
			// 聚合
			sometagks := make([]interface{}, tagkNum)
			for i, tk := range tagks {
				sometagks[i] = tags[tk]
			}
			for _, tk := range aggrTagks {
				for i := 0; i < tagkNum; i++ {
					if tk == tagks[i] {
						sometagks[i] = "<all>"
						break
					}
				}
			}
			summarizedTags := fmt.Sprintf(formator, sometagks...)
			counterEAggregator{}.addSummarizeAggregator(summarizedTags, self, newAggrs)
		}
	}
}
