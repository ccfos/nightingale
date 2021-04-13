package statsd

import (
	"fmt"
	"sort"
	"strconv"
)

type counterAggregator struct {
	Counter float64
}

func (self *counterAggregator) new(aggregatorNames []string) (aggregator, error) {
	if len(aggregatorNames) < 1 || aggregatorNames[0] != "c" {
		return nil, BadAggregatorNameError
	}
	return &counterAggregator{}, nil
}

// counter类型可以接受一个或多个(并包模式下) value, 没有statusCode字段, 不在sdk做并包
// 形如 10{"\u2318"}1{"\u2318"}20
func (self *counterAggregator) collect(values []string, metric string, argLines string) error {
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
		self.Counter += delta
	}

	return nil
}

func (self *counterAggregator) dump(points []*Point, timestamp int64,
	tags map[string]string, metric, argLines string) ([]*Point, error) {

	points = append(points, &Point{
		Name:      metric + ".counter",
		Timestamp: timestamp,
		Tags:      tags,
		Value:     self.Counter,
	})
	return points, nil
}

func (self *counterAggregator) summarize(nsmetric, argLines string, newAggrs map[string]aggregator) {
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

	self.doAggr(tags, newAggrs)

	// 本机聚合

	return
}

func (self *counterAggregator) merge(toMerge aggregator) (aggregator, error) {
	that := toMerge.(*counterAggregator)
	self.Counter += that.Counter
	return self, nil
}

func (self *counterAggregator) toMap() (map[string]interface{}, error) {
	return map[string]interface{}{
		"__aggregator__": "counter",
		"counter":        self.Counter,
	}, nil
}

func (self counterAggregator) fromMap(serialized map[string]interface{}) (aggregator, error) {
	return &counterAggregator{Counter: serialized["counter"].(float64)}, nil
}

// internals
func (self counterAggregator) addSummarizeAggregator(argLines string, toMerge *counterAggregator, newAggrs map[string]aggregator) {
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

func (self *counterAggregator) clone() (aggregator, error) {
	maps, err := self.toMap()
	if err != nil {
		return nil, err
	}

	aggr, err := counterAggregator{}.fromMap(maps)
	if err != nil {
		return nil, err
	}

	return aggr, nil
}

func (self *counterAggregator) doAggr(tags map[string]string, newAggrs map[string]aggregator, aggrTagksList ...[][]string) {
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
	formator += "c"

	// 聚合所有维度
	ntagvs_all := make([]interface{}, tagkNum)
	for i := 0; i < tagkNum; i++ {
		ntagvs_all[i] = "<all>"
	}
	summarizedTags := fmt.Sprintf(formator, ntagvs_all...)

	counterAggregator{}.addSummarizeAggregator(summarizedTags, self, newAggrs)

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
			counterAggregator{}.addSummarizeAggregator(summarizedTags, self, newAggrs)
		}
	}
}
