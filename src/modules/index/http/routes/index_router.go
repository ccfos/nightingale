package routes

import (
	"fmt"

	"github.com/didi/nightingale/src/modules/index/cache"
	"github.com/didi/nightingale/src/toolkits/http/render"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type EndpointsRecv struct {
	Endpoints []string `json:"endpoints"`
}

type MetricList struct {
	Metrics []string `json:"metrics"`
}

func GetMetrics(c *gin.Context) {
	stats.Counter.Set("metric.qp10s", 1)
	recv := EndpointsRecv{}
	errors.Dangerous(c.ShouldBindJSON(&recv))

	m := make(map[string]struct{})
	resp := MetricList{}
	for _, endpoint := range recv.Endpoints {
		metrics := cache.IndexDB.GetMetricsBy(endpoint)
		for _, metric := range metrics {
			if _, exists := m[metric]; !exists {
				m[metric] = struct{}{}
				resp.Metrics = append(resp.Metrics, metric)
			}
		}
	}

	render.Data(c, resp, nil)
}

type EndpointMetricRecv struct {
	Endpoints []string `json:"endpoints"`
	Metrics   []string `json:"metrics"`
}

func DelMetrics(c *gin.Context) {
	recv := EndpointMetricRecv{}
	errors.Dangerous(c.ShouldBindJSON(&recv))

	for _, endpoint := range recv.Endpoints {
		if metricIndexMap, exists := cache.IndexDB.GetMetricIndexMap(endpoint); exists {
			for _, metric := range recv.Metrics {
				metricIndexMap.DelMetric(metric)
			}
		}
	}

	render.Data(c, "ok", nil)
}

type IndexTagkvResp struct {
	Endpoints []string         `json:"endpoints"`
	Metric    string           `json:"metric"`
	Tagkv     []*cache.TagPair `json:"tagkv"`
}

func DelCounter(c *gin.Context) {
	recv := IndexTagkvResp{}
	errors.Dangerous(c.ShouldBindJSON(&recv))

	for _, endpoint := range recv.Endpoints {
		metricIndex, exists := cache.IndexDB.GetMetricIndex(endpoint, recv.Metric)
		if !exists {
			continue
		}

		for _, tagPair := range recv.Tagkv {
			for _, v := range tagPair.Values {
				metricIndex.TagkvMap.DelTag(tagPair.Key, v)
			}
		}
	}

	render.Data(c, "ok", nil)
}

func GetTagPairs(c *gin.Context) {
	stats.Counter.Set("tag.qp10s", 1)
	recv := EndpointMetricRecv{}
	errors.Dangerous(c.ShouldBindJSON(&recv))

	resp := []*IndexTagkvResp{}

	for _, metric := range recv.Metrics {
		tagkvFilter := make(map[string]map[string]struct{})
		tagkvs := []*cache.TagPair{}

		for _, endpoint := range recv.Endpoints {
			metricIndex, exists := cache.IndexDB.GetMetricIndex(endpoint, metric)
			if !exists {
				logger.Debugf("index not found by %s %s", endpoint, metric)
				stats.Counter.Set("query.tag.miss", 1)
				continue
			}

			tagkvMap := metricIndex.TagkvMap.GetTagkvMap()

			for tagk, tagvs := range tagkvMap {
				tagvFilter, exists := tagkvFilter[tagk]
				if !exists {
					tagvFilter = make(map[string]struct{})
				}

				for _, tagv := range tagvs {
					if _, exists := tagvFilter[tagv]; !exists {
						tagvFilter[tagv] = struct{}{}
					}
				}

				tagkvFilter[tagk] = tagvFilter
			}
		}

		for tagk, tagvFilter := range tagkvFilter {
			tagvs := []string{}
			for v := range tagvFilter {
				tagvs = append(tagvs, v)
			}
			tagkv := &cache.TagPair{
				Key:    tagk,
				Values: tagvs,
			}
			tagkvs = append(tagkvs, tagkv)
		}

		TagkvResp := IndexTagkvResp{
			Endpoints: recv.Endpoints,
			Metric:    metric,
			Tagkv:     tagkvs,
		}
		resp = append(resp, &TagkvResp)
	}
	render.Data(c, resp, nil)
}

type GetIndexByFullTagsRecv struct {
	Endpoints []string         `json:"endpoints"`
	Metric    string           `json:"metric"`
	Tagkv     []*cache.TagPair `json:"tagkv"`
}

type GetIndexByFullTagsResp struct {
	Endpoints []string `json:"endpoints"`
	Metric    string   `json:"metric"`
	Tags      []string `json:"tags"`
	Step      int      `json:"step"`
	DsType    string   `json:"dstype"`
}

func GetIndexByFullTags(c *gin.Context) {
	stats.Counter.Set("counter.qp10s", 1)

	recv := []GetIndexByFullTagsRecv{}
	errors.Dangerous(c.ShouldBindJSON(&recv))

	tagFilter := make(map[string]struct{})
	tagsList := []string{}

	var resp []GetIndexByFullTagsResp

	for _, r := range recv {
		metric := r.Metric
		tagkv := r.Tagkv
		step := 0
		dsType := ""

		for _, endpoint := range r.Endpoints {
			if endpoint == "" {
				logger.Debugf("非法请求: endpoint字段缺失:%v", r)
				stats.Counter.Set("query.counter.miss", 1)

				continue
			}
			if metric == "" {
				logger.Debugf("非法请求: metric字段缺失:%v", r)
				stats.Counter.Set("query.counter.miss", 1)
				continue
			}

			metricIndex, exists := cache.IndexDB.GetMetricIndex(endpoint, metric)
			if !exists {
				logger.Debugf("not found index by endpoint:%s metric:%v", endpoint, metric)
				stats.Counter.Set("query.counter.miss", 1)
				continue
			}

			if step == 0 || dsType == "" {
				step = metricIndex.Step
				dsType = metricIndex.DsType
			}

			countersMap := metricIndex.CounterMap.GetCounters()

			tagPairs := cache.GetSortTags(cache.TagPairToMap(tagkv))
			tags := cache.GetAllCounter(tagPairs)

			for _, tag := range tags {
				//校验和tag有关的counter是否存在，如果一个指标，比如port.listen有name=uic,port=8056和name=hsp,port=8002。避免产生4个曲线
				if _, exists := countersMap[tag]; !exists {
					stats.Counter.Set("query.counter.miss", 1)
					logger.Debugf("not found counters byendpoint:%s metric:%v tags:%v\n", endpoint, metric, tag)
					continue
				}

				if _, exists := tagFilter[tag]; !exists {
					tagsList = append(tagsList, tag)
					tagFilter[tag] = struct{}{}
				}
			}
		}

		resp = append(resp, GetIndexByFullTagsResp{
			Endpoints: r.Endpoints,
			Metric:    r.Metric,
			Tags:      tagsList,
			Step:      step,
			DsType:    dsType,
		})
	}

	render.Data(c, resp, nil)
}

type CludeRecv struct {
	Endpoints []string         `json:"endpoints"`
	Metric    string           `json:"metric"`
	Include   []*cache.TagPair `json:"include"`
	Exclude   []*cache.TagPair `json:"exclude"`
}

type XcludeResp struct {
	Endpoint string   `json:"endpoint"`
	Metric   string   `json:"metric"`
	Tags     []string `json:"tags"`
	Step     int      `json:"step"`
	DsType   string   `json:"dstype"`
}

func GetIndexByClude(c *gin.Context) {
	stats.Counter.Set("xclude.qp10s", 1)

	recv := []CludeRecv{}
	errors.Dangerous(c.ShouldBindJSON(&recv))

	var resp []XcludeResp

	for _, r := range recv {
		metric := r.Metric
		includeList := r.Include
		excludeList := r.Exclude
		step := 0
		dsType := ""
		tagList := []string{}
		tagFilter := make(map[string]struct{})

		for _, endpoint := range r.Endpoints {
			if endpoint == "" {
				logger.Debugf("非法请求: endpoint字段缺失:%v", r)
				stats.Counter.Set("xclude.miss", 1)
				continue
			}
			if metric == "" {
				logger.Debugf("非法请求: metric字段缺失:%v", r)
				stats.Counter.Set("xclude.miss", 1)

				continue
			}

			metricIndex, exists := cache.IndexDB.GetMetricIndex(endpoint, metric)
			if !exists {

				resp = append(resp, XcludeResp{
					Endpoint: endpoint,
					Metric:   metric,
					Tags:     tagList,
					Step:     step,
					DsType:   dsType,
				})
				logger.Debugf("not found index by endpoint:%s metric:%v\n", endpoint, metric)
				stats.Counter.Set("xclude.miss", 1)

				continue
			}

			if step == 0 || dsType == "" {
				step = metricIndex.Step
				dsType = metricIndex.DsType
			}

			//校验实际tag组合成的counter是否存在，如果一个指标，比如port.listen有name=uic,port=8056和name=hsp,port=8002。避免产生4个曲线
			counterMap := metricIndex.CounterMap.GetCounters()

			var err error
			var tags []string
			if len(includeList) == 0 && len(excludeList) == 0 {
				for counter := range counterMap {
					tagList = append(tagList, counter)
				}
				resp = append(resp, XcludeResp{
					Endpoint: endpoint,
					Metric:   metric,
					Tags:     tagList,
					Step:     step,
					DsType:   dsType,
				})
				continue
			} else {
				tags, err = cache.IndexDB.GetIndexByClude(endpoint, metric, includeList, excludeList)
				if err != nil {
					logger.Warning(err)
					continue
				}
			}

			for _, tag := range tags {
				if tag == "" { //过滤掉空字符串
					continue
				}

				//校验实际tag组合成的counter是否存在，如果一个指标，比如port.listen有name=uic,port=8056和name=hsp,port=8002。避免产生4个曲线
				if _, exists := counterMap[tag]; !exists {
					logger.Debugf("not found counters by endpoint:%s metric:%v tags:%v\n", endpoint, metric, tag)
					stats.Counter.Set("xclude.miss", 1)
					continue
				}

				if _, exists := tagFilter[tag]; !exists {
					tagList = append(tagList, tag)
					tagFilter[tag] = struct{}{}
				}
			}

			resp = append(resp, XcludeResp{
				Endpoint: endpoint,
				Metric:   metric,
				Tags:     tagList,
				Step:     step,
				DsType:   dsType,
			})
		}
	}

	render.Data(c, resp, nil)
}

func DumpIndex(c *gin.Context) {
	err := cache.Persist("normal")
	errors.Dangerous(err)

	render.Data(c, "ok", nil)
}

func GetIdxFile(c *gin.Context) {
	err := cache.Persist("download")
	errors.Dangerous(err)

	traGz := fmt.Sprintf("%s/%s", cache.Config.PersistDir, "db.tar.gz")
	c.File(traGz)
}
