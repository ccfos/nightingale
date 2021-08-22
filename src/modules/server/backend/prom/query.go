// Copyright 2017 Xiaomi, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prom

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/prometheus/common/model"

	"github.com/toolkits/pkg/concurrent/semaphore"
)

func (prom *PromDataSource) QueryData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse {
	var resp []*dataobj.TsdbQueryResponse

	sema := semaphore.NewSemaphore(20)
	wg := &sync.WaitGroup{}
	lock := &sync.Mutex{}

	for _, input := range inputs {
		for _, counter := range input.Counters {
			wg.Add(1)
			sema.Acquire()
			go func(q dataobj.QueryData, c string) {
				defer wg.Done()
				defer sema.Release()

				metric, tags := decodeCounter(c)
				if metric == nil {
					return
				}

				selector := prom.newMetricSelector([]string{*metric}, q.Endpoints, q.Nids)
				selector.addLabelList(tags)

				value, err := prom.ReadClients.QueryRange(selector, q.Start, q.End, q.Step)
				if err != nil {
					return
				}

				for idx := range value {
					value[idx].Counter = prom.convertPromMetricName(value[idx].Counter)
				}

				lock.Lock()
				defer lock.Unlock()
				resp = append(resp, value...)
			}(input, counter)

		}
	}

	wg.Wait()
	return resp
}

func (prom *PromDataSource) QueryDataForUI(input dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {
	selector := prom.newMetricSelector([]string{input.Metric}, input.Endpoints, input.Nids)
	selector.addLabelList(input.Tags)

	value, err := prom.ReadClients.QueryRange(selector, input.Start, input.End, input.Step)
	if err != nil {
		return nil
	}

	for idx := range value {
		value[idx].Counter = prom.convertPromMetricName(value[idx].Counter)
	}
	return value
}

func (prom *PromDataSource) QueryMetrics(recv dataobj.EndpointsRecv) *dataobj.MetricResp {
	selector := prom.newMetricSelector(nil, recv.Endpoints, recv.Nids)

	labels, err := prom.ReadClients.LabelValues(model.MetricNameLabel, []metricSelector{selector})
	if err != nil {
		return nil
	}
	if labels == nil || len(labels) <= 0 {
		return nil
	}

	var metrics []string
	for _, l := range labels {
		if !strings.HasPrefix(l, prom.Section.Prefix) {
			continue
		}
		metrics = append(metrics, prom.convertPromMetricName(l))
	}

	return &dataobj.MetricResp{
		Metrics: metrics,
	}
}

func (prom *PromDataSource) QueryTagPairs(recv dataobj.EndpointMetricRecv) []dataobj.IndexTagkvResp {
	if len(recv.Metrics) <= 0 {
		return nil
	}
	if len(recv.Endpoints) <= 0 && len(recv.Nids) <= 0 {
		return nil
	}

	selector := prom.newMetricSelector(recv.Metrics, recv.Endpoints, recv.Nids)
	end := time.Now()
	start := end.AddDate(0, 0, -3)

	metrics, err := prom.ReadClients.GetSeries([]metricSelector{selector}, start.Unix(), end.Unix())
	if err != nil {
		return nil
	}
	if metrics == nil || len(metrics) <= 0 {
		return nil
	}

	smap := make(map[string]metricSelector)
	for _, metric := range metrics {
		if len(metric.Name) <= 0 {
			continue
		}
		name := prom.convertPromMetricName(metric.Name)
		if len(name) <= 0 {
			continue
		}

		s, ok := smap[name]
		if !ok {
			smap[name] = prom.newSelectorByMeta(metric)
			continue
		}

		s.addMetricMeta(metric)
	}

	var ret []dataobj.IndexTagkvResp
	for name, selector := range smap {
		tagkv := dataobj.IndexTagkvResp{
			Metric: name,
		}
		for lnane, lval := range selector {
			if lval == nil || len(lval) <= 0 {
				continue
			}

			if lnane == model.MetricNameLabel {
				continue
			}
			if lnane == LabelEndpoint {
				tagkv.Endpoints = lval.dedup()
				continue
			}
			if lnane == LabelNid {
				tagkv.Nids = lval.dedup()
				continue
			}

			pair := &dataobj.TagPair{
				Key:    lnane,
				Values: lval.dedup(),
			}
			tagkv.Tagkv = append(tagkv.Tagkv, pair)
		}
		ret = append(ret, tagkv)
	}

	return ret
}

func (prom *PromDataSource) QueryIndexByClude(recv []dataobj.CludeRecv) []dataobj.XcludeResp {
	return nil
}

func (prom *PromDataSource) QueryIndexByFullTags(recv []dataobj.IndexByFullTagsRecv) ([]dataobj.IndexByFullTagsResp, int) {
	count := 0
	var ret []dataobj.IndexByFullTagsResp

	for _, r := range recv {
		res := prom.QueryIndexByFullTagsOne(r)
		if res != nil {
			ret = append(ret, *res)
			count += res.Count
		}
	}
	return ret, count
}

func (prom *PromDataSource) QueryIndexByFullTagsOne(recv dataobj.IndexByFullTagsRecv) *dataobj.IndexByFullTagsResp {
	if len(recv.Metric) <= 0 {
		return nil
	}
	if len(recv.Endpoints) <= 0 && len(recv.Nids) <= 0 {
		return nil
	}

	selector := prom.newMetricSelector([]string{recv.Metric}, recv.Endpoints, recv.Nids)
	for _, tagkv := range recv.Tagkv {
		selector.addLabels(tagkv.Key, tagkv.Values)
	}

	metrics, err := prom.ReadClients.GetSeries([]metricSelector{selector}, recv.Start, recv.End)
	if err != nil {
		return nil
	}
	if metrics == nil || len(metrics) <= 0 {
		return nil
	}

	resp := dataobj.IndexByFullTagsResp{
		Metric: recv.Metric,
		Step:   30,
		DsType: GaugeTypeStr,
		Count:  0,
	}

	endpointMap := make(map[string]struct{})
	nidMap := make(map[string]struct{})
	tagsMap := make(map[string]struct{})
	for _, metric := range metrics {
		name := prom.convertPromMetricName(metric.Name)
		if name != recv.Metric {
			continue
		}

		resp.Count++
		if len(metric.Endpoind) > 0 {
			endpointMap[metric.Endpoind] = struct{}{}
		}
		if len(metric.Nid) > 0 {
			endpointMap[metric.Nid] = struct{}{}
		}

		for k, v := range metric.Tags {
			// TODO: trick need remove
			if strings.HasPrefix(k, prom.Section.Prefix) {
				continue
			}
			tag := fmt.Sprintf("%s=%s", k, v)
			tagsMap[tag] = struct{}{}
		}
	}

	for e := range endpointMap {
		resp.Endpoints = append(resp.Endpoints, e)
	}
	for n := range nidMap {
		resp.Nids = append(resp.Nids, n)
	}
	for t := range tagsMap {
		resp.Tags = append(resp.Tags, t)
	}

	return &resp
}

func decodeCounter(counter string) (*string, []string) {
	mt := strings.SplitN(counter, "/", 2)
	lenmt := len(mt)

	var metric string
	if lenmt <= 0 {
		return nil, nil
	}
	if lenmt == 1 {
		metric = mt[0]
		return &metric, nil
	}
	metric = mt[0]

	tagList := strings.Split(mt[1], ",")

	return &metric, tagList
}
