package statsd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type rpcEAggregator struct {
	histogramAggregator
	Counters map[string]float64
	Latencys map[string]float64
}

func (self *rpcEAggregator) new(aggregatorNames []string) (aggregator, error) {
	if len(aggregatorNames) < 1 || aggregatorNames[0] != "rpce" {
		return nil, BadAggregatorNameError
	}

	histogramAggregatorNames := []string{"p99", "p95", "p75", "p50"}
	return &rpcEAggregator{
		histogramAggregator: histogramAggregator{}.newInstence(histogramAggregatorNames),
		Counters:            map[string]float64{},
		Latencys:            map[string]float64{},
	}, nil
}

func (self *rpcEAggregator) collect(values []string, metric string, argLines string) error {
	if len(values) < 1 {
		return fmt.Errorf("bad values")
	}

	for i := range values {
		cvalues := strings.Split(values[i], CodeDelimiter)
		if len(cvalues) < 2 {
			// bad values
			continue
		}

		err := self.histogramAggregator.collect(cvalues[:1], metric, argLines)
		if err != nil {
			return err
		}

		latency, err := strconv.ParseFloat(cvalues[0], 64)
		if err != nil {
			return err
		}

		code := cvalues[1]
		self.Counters[code] += 1

		self.Latencys[code] += latency
	}

	return nil
}

// @input
//		metric: $metric_name(不包含ns)
func (self *rpcEAggregator) dump(points []*Point, timestamp int64,
	tags map[string]string, metric, argLines string) ([]*Point, error) {
	var (
		err error
	)

	// 无数据,则不dump点
	if len(self.Counters) == 0 {
		return points, nil
	}

	// 验证tag信息: 必须存在callee caller
	if _, ok := tags["caller"]; !ok {
		return points, nil
	}

	callee, ok := tags["callee"]
	if !ok {
		return points, nil
	}
	tags["callee"] = Func{}.TrimRpcCallee(callee) // 修改callee字段

	// 带tag的rpc统计, 指标名称调整为 by_tags.$metric
	//if len(tags) > 2 {
	//	metric = fmt.Sprintf("by_tags.%s", metric)
	//}

	totalCount := float64(0)
	totalErrorCount := float64(0)
	for code, count := range self.Counters {
		if !(Func{}.IsOk(code)) {
			myTags := map[string]string{}
			for k, v := range tags {
				myTags[k] = v
			}
			myTags["code"] = code
			points = append(points, &Point{
				Name:      metric + ".error.counter",
				Timestamp: timestamp,
				Tags:      myTags,
				Value:     count,
			})
			totalErrorCount += count
		}
		totalCount += count
	}
	points = append(points, &Point{
		Name:      metric + ".counter",
		Timestamp: timestamp,
		Tags:      tags,
		Value:     totalCount,
	})
	if totalCount > 0 {
		for code, count := range self.Counters {
			myTags := map[string]string{}
			for k, v := range tags {
				myTags[k] = v
			}
			myTags["code"] = code
			points = append(points, &Point{
				Name:      metric + ".code.ratio",
				Timestamp: timestamp,
				Tags:      myTags,
				Value:     count / totalCount * 100,
			})
		}

		points = append(points, &Point{
			Name:      metric + ".error.ratio",
			Timestamp: timestamp,
			Tags:      tags,
			Value:     totalErrorCount / totalCount * 100,
		})
		myTags := map[string]string{}
		for k, v := range tags {
			myTags[k] = v
		}
		myTags["code"] = "<all>"
		points = append(points, &Point{
			Name:      metric + ".error.counter",
			Timestamp: timestamp,
			Tags:      myTags,
			Value:     totalErrorCount,
		})
	}

	// latency
	latencyMetric := fmt.Sprintf("%s.latency", metric)
	{ // avg
		totalLatency := float64(0)
		for _, latency := range self.Latencys {
			totalLatency += latency
		}
		avgLatency := float64(0)
		if totalCount > 0 && totalLatency > 0 {
			avgLatency = totalLatency / totalCount
		}

		myTags := map[string]string{}
		for k, v := range tags {
			myTags[k] = v
		}
		myTags["percentile"] = "avg"

		points = append(points, &Point{
			Name:      latencyMetric,
			Timestamp: timestamp,
			Tags:      myTags,
			Value:     avgLatency,
		})
	}
	points, err = self.histogramAggregator.dump(points, timestamp, tags, latencyMetric, argLines) // percentile

	return points, err
}

func (self *rpcEAggregator) summarize(nsmetric, argLines string, newAggrs map[string]aggregator) {
	items, _ := Func{}.TranslateMetricLine(nsmetric)
	//ns := items[0]
	metric := items[1]

	tags, _, err := Func{}.TranslateArgLines(argLines)
	if err != nil {
		return
	}

	// rpc_dirpc_call & rpc_dirpc_called
	if metric == MetricToBeSummarized_DirpcCallConst || metric == MetricToBeSummarized_DirpcCalledConst {
		if len(tags) != 5 {
			return
		}
		callee, _ := tags["callee"]
		calleef, _ := tags["callee-func"]
		caller, _ := tags["caller"]
		callerf, _ := tags["caller-func"]
		su, _ := tags["su"]
		if !(caller != "" && callerf != "" && callee != "" && calleef != "" && su != "") {
			return
		}

		formator := "callee=%s\ncallee-func=%s\ncaller=%s\ncaller-func=%s\nsu=%s\nrpce"
		if calleef != "<all>" {
			summarizedCalleef := fmt.Sprintf(formator, callee, "<all>", caller, callerf, su)
			rpcEAggregator{}.addSummarizeAggregator(summarizedCalleef, self, newAggrs)
		}
		if callerf != "<all>" {
			summarizedCallerf := fmt.Sprintf(formator, callee, calleef, caller, "<all>", su)
			rpcEAggregator{}.addSummarizeAggregator(summarizedCallerf, self, newAggrs)
		}
		if calleef != "<all>" && callerf != "<all>" {
			summarizedCalleefCallerf := fmt.Sprintf(formator, callee, "<all>", caller, "<all>", su)
			rpcEAggregator{}.addSummarizeAggregator(summarizedCalleefCallerf, self, newAggrs)
		}

		return
	}

	// rpcdisf
	if metric == MetricToBeSummarized_RpcdisfConst {
		if len(tags) != 7 {
			return
		}
		callee, _ := tags["callee"]
		calleec, _ := tags["callee-cluster"]
		calleef, _ := tags["callee-func"]
		caller, _ := tags["caller"]
		callerc, _ := tags["caller-cluster"]
		callerf, _ := tags["caller-func"]
		su, _ := tags["su"]
		if !(caller != "" && callerc != "" && callerf != "" &&
			callee != "" && calleec != "" && calleef != "" && su != "") {
			return
		}

		formator := "callee=%s\ncallee-cluster=%s\ncallee-func=%s\ncaller=%s\ncaller-cluster=%s\ncaller-func=%s\nsu=%s\nrpce"
		if calleef != "<all>" {
			summarizedCalleef := fmt.Sprintf(formator, callee, calleec, "<all>", caller, callerc, callerf, su)
			rpcEAggregator{}.addSummarizeAggregator(summarizedCalleef, self, newAggrs)
		}
		if callerf != "<all>" {
			summarizedCallerf := fmt.Sprintf(formator, callee, calleec, calleef, caller, callerc, "<all>", su)
			rpcEAggregator{}.addSummarizeAggregator(summarizedCallerf, self, newAggrs)
		}
		summarizedCalleefCallerf := fmt.Sprintf(formator, callee, calleec, "<all>", caller, callerc, "<all>", su)
		rpcEAggregator{}.addSummarizeAggregator(summarizedCalleefCallerf, self, newAggrs)

		return
	}

	// rpcdfe
	if metric == MetricToBeSummarized_RpcdfeConst {
		tagks := make([]string, 0)
		for k, _ := range tags {
			tagks = append(tagks, k)
		}

		tagkLen := len(tagks)
		if tagkLen < 3 {
			return
		}
		sort.Strings(tagks)

		callee, _ := tags["callee"]
		caller, _ := tags["caller"]
		service, _ := tags["service"]
		if !(callee != "" && caller != "" && service != "") {
			return
		}

		// 单独聚合callee caller service schema
		for k, v := range tags {
			if (k == "callee" && v != "<all>") || (k == "caller" && v != "<all>") ||
				(k == "service" && v != "<all>") || (k == "schema" && v != "<all>") {

				formator := ""
				for i := 0; i < tagkLen; i++ {
					formator += tagks[i] + "=%s\n"
				}
				formator += "rpce"

				// 聚合所有维度
				ntagvs_all := make([]interface{}, tagkLen)
				for i := 0; i < tagkLen; i++ {
					if tagks[i] == k {
						ntagvs_all[i] = "<all>"
					} else {
						ntagvs_all[i] = tags[tagks[i]]
					}
				}
				summarizedTags := fmt.Sprintf(formator, ntagvs_all...)
				rpcEAggregator{}.addSummarizeAggregator(summarizedTags, self, newAggrs)
			}
		}
		// 默认聚合所有tag
		self.doAggr(tags, newAggrs)
		return
	}

	// 黑名单

	// 只做默认聚合
	self.doAggr(tags, newAggrs)

	// 本机聚合

	return
}

func (self *rpcEAggregator) merge(toMerge aggregator) (aggregator, error) {
	that, ok := toMerge.(*rpcEAggregator)
	if !ok {
		return nil, BadSummarizeAggregatorError
	}

	_, err := self.histogramAggregator.merge(&that.histogramAggregator)
	if err != nil {
		return nil, err
	}

	for k, v2 := range that.Counters {
		_, found := self.Counters[k]
		if found {
			self.Counters[k] += v2
		} else {
			self.Counters[k] = v2
		}
	}
	for k, v2 := range that.Latencys {
		_, found := self.Latencys[k]
		if found {
			self.Latencys[k] += v2
		} else {
			self.Latencys[k] = v2
		}
	}
	return self, nil
}

func (self *rpcEAggregator) toMap() (map[string]interface{}, error) {
	counters := map[string]interface{}{}
	for k, v := range self.Counters {
		counters[k] = v
	}

	latencys := map[string]interface{}{}
	for k, v := range self.Latencys {
		latencys[k] = v
	}

	hm, err := self.histogramAggregator.toMap()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"__aggregator__": "rpce",
		"counters":       counters,
		"latencys":       latencys,
		"histogram":      hm,
	}, nil
}

func (self rpcEAggregator) fromMap(serialized map[string]interface{}) (aggregator, error) {
	aggregator := &rpcEAggregator{Counters: map[string]float64{}, Latencys: map[string]float64{}}
	counters := (serialized["counters"]).(map[string]interface{})
	for k, v := range counters {
		aggregator.Counters[k] = v.(float64)
	}

	latencys := (serialized["latencys"]).(map[string]interface{})
	for k, v := range latencys {
		aggregator.Latencys[k] = v.(float64)
	}

	histogram := (serialized["histogram"]).(map[string]interface{})
	hm, err := self.histogramAggregator.fromMap(histogram)
	if err != nil {
		return nil, err
	}

	hmaggr, ok := hm.(*histogramAggregator)
	if !ok {
		return nil, BadDeserializeError
	}

	aggregator.histogramAggregator = *hmaggr
	return aggregator, nil
}

// internal functions
func (self rpcEAggregator) addSummarizeAggregator(argLines string, toMerge *rpcEAggregator, newAggrs map[string]aggregator) {
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

func (self *rpcEAggregator) clone() (aggregator, error) {
	maps, err := self.toMap()
	if err != nil {
		return nil, err
	}

	aggr, err := rpcEAggregator{}.fromMap(maps)
	if err != nil {
		return nil, err
	}

	return aggr, nil
}

func (self *rpcEAggregator) doAggr(tags map[string]string, newAggrs map[string]aggregator, aggrTagksList ...[][]string) {
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
	formator += "rpce"

	// 聚合所有维度
	ntagvs_all := make([]interface{}, tagkNum)
	for i := 0; i < tagkNum; i++ {
		ntagvs_all[i] = "<all>"
	}
	summarizedTags := fmt.Sprintf(formator, ntagvs_all...)
	rpcEAggregator{}.addSummarizeAggregator(summarizedTags, self, newAggrs)

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
			rpcEAggregator{}.addSummarizeAggregator(summarizedTags, self, newAggrs)
		}
	}
}
