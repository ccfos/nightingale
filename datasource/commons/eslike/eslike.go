package eslike

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	"github.com/bitly/go-simplejson"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/mitchellh/mapstructure"
	"github.com/olivere/elastic/v7"
	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
)

type Query struct {
	Ref        string     `json:"ref" mapstructure:"ref"`
	Index      string     `json:"index" mapstructure:"index"`
	Filter     string     `json:"filter" mapstructure:"filter"`
	MetricAggr MetricAggr `json:"value" mapstructure:"value"`
	GroupBy    []GroupBy  `json:"group_by" mapstructure:"group_by"`
	DateField  string     `json:"date_field" mapstructure:"date_field"`
	Interval   int64      `json:"interval" mapstructure:"interval"`
	Start      int64      `json:"start" mapstructure:"start"`
	End        int64      `json:"end" mapstructure:"end"`
	P          int        `json:"page" mapstructure:"page"`           // 页码
	Limit      int        `json:"limit" mapstructure:"limit"`         // 每页个数
	Ascending  bool       `json:"ascending" mapstructure:"ascending"` // 按照DataField排序

	Timeout  int `json:"timeout" mapstructure:"timeout"`
	MaxShard int `json:"max_shard" mapstructure:"max_shard"`
}

type MetricAggr struct {
	Field string `json:"field" mapstructure:"field"`
	Func  string `json:"func" mapstructure:"func"`
	Ref   string `json:"ref" mapstructure:"ref"` // 变量名，A B C
}

type GroupBy struct {
	Cate        GroupByCate `json:"cate" mapstructure:"cate"` // 分组类型
	Field       string      `json:"field" mapstructure:"field"`
	MinDocCount int64       `json:"min_doc_count" mapstructure:"min_doc_count"`
	Order       string      `json:"order" mapstructure:"order"`
	OrderBy     string      `json:"order_by" mapstructure:"order_by"`
	Size        int         `json:"size" mapstructure:"size"`

	Params   []Param `json:"params" mapstructure:"params"`     // 类型是 filter 时使用
	Interval int64   `json:"interval" mapstructure:"interval"` // 分组间隔
}

type SearchFunc func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error)
type QueryFieldsFunc func(indices []string) ([]string, error)

// 分组类型
type GroupByCate string

const (
	Filters  GroupByCate = "filters"
	Histgram GroupByCate = "histgram"
	Terms    GroupByCate = "terms"
)

// 参数
type Param struct {
	Alias string `json:"alias,omitempty"` // 别名，a=b的形式，filter 特有
	Query string `json:"query,omitempty"` // 查询条件，filter 特有
}

type MetricPtr struct {
	Data map[string][][]float64
}

func IterGetMap(m, ret map[string]interface{}, prefixKey string) {
	for k, v := range m {
		switch v.(type) {
		case map[string]interface{}:
			var key string
			if prefixKey != "" {
				key = fmt.Sprintf("%s.%s", prefixKey, k)
			} else {
				key = k
			}
			IterGetMap(v.(map[string]interface{}), ret, key)
		default:
			ret[prefixKey+"."+k] = []interface{}{v}
		}
	}
}

func TransferData(metric, ref string, m map[string][][]float64) []models.DataResp {
	var datas []models.DataResp

	for k, v := range m {
		data := models.DataResp{
			Ref:    ref,
			Metric: make(model.Metric),
			Labels: k,
			Values: v,
		}

		data.Metric["__name__"] = model.LabelValue(metric)
		labels := strings.Split(k, "--")
		for _, label := range labels {
			arr := strings.SplitN(label, "=", 2)
			if len(arr) == 2 {
				data.Metric[model.LabelName(arr[0])] = model.LabelValue(arr[1])
			}
		}
		datas = append(datas, data)
	}

	for i := 0; i < len(datas); i++ {
		for k, v := range datas[i].Metric {
			if k == "__name__" {
				datas[i].Metric[k] = model.LabelValue(ref) + "_" + v
			}
		}
	}

	return datas
}

func GetQueryString(filter string, q *elastic.RangeQuery) *elastic.BoolQuery {
	var queryString *elastic.BoolQuery
	if filter != "" {
		if strings.Contains(filter, ":") || strings.Contains(filter, "AND") || strings.Contains(filter, "OR") || strings.Contains(filter, "NOT") {
			queryString = elastic.NewBoolQuery().Must(elastic.NewQueryStringQuery(filter)).Filter(q)
		} else {
			queryString = elastic.NewBoolQuery().Filter(elastic.NewMultiMatchQuery(filter).Lenient(true).Type("phrase")).Filter(q)
		}
	} else {
		queryString = elastic.NewBoolQuery().Should(q)
	}

	return queryString
}

func getUnixTs(timeStr string) int64 {
	ts, err := strconv.ParseInt(timeStr, 10, 64)
	if err == nil {
		return ts
	}

	parsedTime, err := dateparse.ParseAny(timeStr)
	if err != nil {
		logger.Error("failed to ParseAny: ", err)
		return 0
	}
	return parsedTime.UnixMilli()
}

func GetBuckts(labelKey string, keys []string, arr []interface{}, metrics *MetricPtr, labels string, ts int64, f string) {
	var err error
	bucketsKey := ""
	if len(keys) > 0 {
		bucketsKey = keys[0]
	}

	newlabels := ""
	for i := 0; i < len(arr); i++ {
		tmp := arr[i].(map[string]interface{})
		keyAsString, getTs := tmp["key_as_string"]
		if getTs {
			ts = getUnixTs(keyAsString.(string))
		}
		keyValue := tmp["key"]
		switch keyValue.(type) {
		case json.Number, string:
			if !getTs {
				if labels != "" {
					newlabels = fmt.Sprintf("%s--%s=%v", labels, labelKey, keyValue)
				} else {
					newlabels = fmt.Sprintf("%s=%v", labelKey, keyValue)
				}
			}
		default:
			continue
		}

		var finalValue float64
		if len(keys) == 0 { // 计算 doc_count 的情况
			count := tmp["doc_count"]
			finalValue, err = count.(json.Number).Float64()
			if err != nil {
				logger.Warningf("labelKey:%s get value error:%v", labelKey, err)
			}
			newValues := []float64{float64(ts / 1000), finalValue}
			metrics.Data[newlabels] = append(metrics.Data[newlabels], newValues)
			continue
		}

		innerBuckets, exists := tmp[bucketsKey]
		if !exists {
			continue
		}

		nextBucketsArr, exists := innerBuckets.(map[string]interface{})["buckets"]
		if exists {
			if len(keys[1:]) >= 1 {
				GetBuckts(bucketsKey, keys[1:], nextBucketsArr.([]interface{}), metrics, newlabels, ts, f)
			} else {
				GetBuckts(bucketsKey, []string{}, nextBucketsArr.([]interface{}), metrics, newlabels, ts, f)
			}
		} else {

			// doc_count
			if f == "count" || f == "nodata" {
				count := tmp["doc_count"]
				finalValue, err = count.(json.Number).Float64()
				if err != nil {
					logger.Warningf("get %v value error:%v", count, err)
				}
			} else {
				values, exists := innerBuckets.(map[string]interface{})["value"]
				if exists {
					switch values.(type) {
					case json.Number:
						value, err := values.(json.Number).Float64()
						if err != nil {
							logger.Warningf("labelKey:%s get value error:%v", labelKey, err)
						}
						finalValue = value
					}
				} else {
					switch values.(type) {
					case map[string]interface{}:
						var err error
						values := innerBuckets.(map[string]interface{})["values"]
						for _, v := range values.(map[string]interface{}) {
							finalValue, err = v.(json.Number).Float64()
							if err != nil {
								logger.Warningf("labelKey:%s get value error:%v", labelKey, err)
							}
						}
					default:
						values := innerBuckets.(map[string]interface{})["values"]
						for _, v := range values.(map[string]interface{}) {
							// Todo 修复 v is nil 导致 panic 情况
							finalValue, err = v.(json.Number).Float64()
							if err != nil {
								logger.Warningf("labelKey:%s get value error:%v", labelKey, err)
							}
						}
					}
				}
			}

			if _, exists := metrics.Data[newlabels]; !exists {
				metrics.Data[newlabels] = [][]float64{}
			}

			newValues := []float64{float64(ts / 1000), finalValue}
			metrics.Data[newlabels] = append(metrics.Data[newlabels], newValues)
		}
	}
}

func MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	param := new(Query)
	if err := mapstructure.Decode(query, param); err != nil {
		return nil, err
	}

	for i := 0; i < len(eventTags); i++ {
		eventTags[i] = strings.Replace(eventTags[i], "=", ":", 1)
	}

	if len(eventTags) > 0 {
		if param.Filter == "" {
			param.Filter = strings.Join(eventTags, " AND ")
		} else {
			param.Filter = param.Filter + " AND " + strings.Join(eventTags, " AND ")
		}
	}

	param.Start = start
	param.End = end

	return param, nil
}

func MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	param := new(Query)
	if err := mapstructure.Decode(query, param); err != nil {
		return nil, err
	}

	for i := 0; i < len(eventTags); i++ {
		eventTags[i] = strings.Replace(eventTags[i], "=", ":", 1)
	}

	if len(eventTags) > 0 {
		if param.Filter == "" {
			param.Filter = strings.Join(eventTags, " AND ")
		} else {
			param.Filter = param.Filter + " AND " + strings.Join(eventTags, " AND ")
		}
	}
	param.Start = start
	param.End = end

	return param, nil
}

func QueryData(ctx context.Context, queryParam interface{}, cliTimeout int64, version string, search SearchFunc) ([]models.DataResp, error) {
	param := new(Query)
	if err := mapstructure.Decode(queryParam, param); err != nil {
		return nil, err
	}

	if param.Timeout == 0 {
		param.Timeout = int(cliTimeout) / 1000
	}

	if param.Interval == 0 {
		param.Interval = 60
	}

	if param.MaxShard < 1 {
		param.MaxShard = 5
	}

	if param.DateField == "" {
		param.DateField = "@timestamp"
	}

	indexArr := strings.Split(param.Index, ",")
	q := elastic.NewRangeQuery(param.DateField)
	now := time.Now().Unix()
	var start, end int64
	if param.End != 0 && param.Start != 0 {
		end = param.End - param.End%param.Interval
		start = param.Start - param.Start%param.Interval
	} else {
		end = now
		start = end - param.Interval
	}

	delay, ok := ctx.Value("delay").(int64)
	if ok && delay != 0 {
		end = end - delay
		start = start - delay
	}

	q.Gte(time.Unix(start, 0).UnixMilli())
	q.Lte(time.Unix(end, 0).UnixMilli())
	q.Format("epoch_millis")

	field := param.MetricAggr.Field
	groupBys := param.GroupBy

	queryString := GetQueryString(param.Filter, q)

	var aggr elastic.Aggregation
	switch param.MetricAggr.Func {
	case "avg":
		aggr = elastic.NewAvgAggregation().Field(field)
	case "max":
		aggr = elastic.NewMaxAggregation().Field(field)
	case "min":
		aggr = elastic.NewMinAggregation().Field(field)
	case "sum":
		aggr = elastic.NewSumAggregation().Field(field)
	case "count":
		aggr = elastic.NewValueCountAggregation().Field(field)
	case "p90":
		aggr = elastic.NewPercentilesAggregation().Percentiles(90).Field(field)
	case "p95":
		aggr = elastic.NewPercentilesAggregation().Percentiles(95).Field(field)
	case "p99":
		aggr = elastic.NewPercentilesAggregation().Percentiles(99).Field(field)
	case "median":
		aggr = elastic.NewPercentilesAggregation().Percentiles(50).Field(field)
	default:
		return nil, fmt.Errorf("func %s not support", param.MetricAggr.Func)
	}

	tsAggr := elastic.NewDateHistogramAggregation().
		Field(param.DateField).
		MinDocCount(1)

	if strings.HasPrefix(version, "7") {
		tsAggr.FixedInterval(fmt.Sprintf("%ds", param.Interval))
	} else {
		// 兼容 7.0 以下的版本
		// OpenSearch 也使用这个字段
		tsAggr.Interval(fmt.Sprintf("%ds", param.Interval))
	}

	// group by
	var groupByAggregation elastic.Aggregation
	if len(groupBys) > 0 {
		groupBy := groupBys[0]

		if groupBy.MinDocCount == 0 {
			groupBy.MinDocCount = 1
		}

		if groupBy.Size == 0 {
			groupBy.Size = 300
		}

		switch groupBy.Cate {
		case Terms:
			if param.MetricAggr.Func != "count" {
				groupByAggregation = elastic.NewTermsAggregation().Field(groupBy.Field).SubAggregation(field, aggr).OrderByKeyDesc().Size(groupBy.Size).MinDocCount(int(groupBy.MinDocCount))
			} else {
				groupByAggregation = elastic.NewTermsAggregation().Field(groupBy.Field).OrderByKeyDesc().Size(groupBy.Size).MinDocCount(int(groupBy.MinDocCount))
			}
		case Histgram:
			if param.MetricAggr.Func != "count" {
				groupByAggregation = elastic.NewHistogramAggregation().Field(groupBy.Field).Interval(float64(groupBy.Interval)).SubAggregation(field, aggr)
			} else {
				groupByAggregation = elastic.NewHistogramAggregation().Field(groupBy.Field).Interval(float64(groupBy.Interval))
			}
		case Filters:
			for _, filterParam := range groupBy.Params {
				if param.MetricAggr.Func != "count" {
					groupByAggregation = elastic.NewFilterAggregation().Filter(elastic.NewTermQuery(filterParam.Query, "true")).SubAggregation(field, aggr)
				} else {
					groupByAggregation = elastic.NewFilterAggregation().Filter(elastic.NewTermQuery(filterParam.Query, "true"))
				}
			}
		}

		for i := 1; i < len(groupBys); i++ {
			groupBy := groupBys[i]

			if groupBy.MinDocCount == 0 {
				groupBy.MinDocCount = 1
			}

			if groupBy.Size == 0 {
				groupBy.Size = 300
			}

			switch groupBy.Cate {
			case Terms:
				groupByAggregation = elastic.NewTermsAggregation().Field(groupBy.Field).SubAggregation(groupBys[i-1].Field, groupByAggregation).OrderByKeyDesc().Size(groupBy.Size).MinDocCount(int(groupBy.MinDocCount))
			case Histgram:
				groupByAggregation = elastic.NewHistogramAggregation().Field(groupBy.Field).Interval(float64(groupBy.Interval)).SubAggregation(groupBys[i-1].Field, groupByAggregation)
			case Filters:
				for _, filterParam := range groupBy.Params {
					groupByAggregation = elastic.NewFilterAggregation().Filter(elastic.NewTermQuery(filterParam.Query, "true")).SubAggregation(groupBys[i-1].Field, groupByAggregation)
				}
			}
		}

		tsAggr.SubAggregation(groupBys[len(groupBys)-1].Field, groupByAggregation)
	} else if param.MetricAggr.Func != "count" {
		tsAggr.SubAggregation(field, aggr)
	}

	source, _ := queryString.Source()
	b, _ := json.Marshal(source)
	logger.Debugf("query_data q:%+v tsAggr:%+v query_string:%s", param, tsAggr, string(b))

	searchSource := elastic.NewSearchSource().
		Query(queryString).
		Aggregation("ts", tsAggr)

	searchSourceString, err := searchSource.Source()
	if err != nil {
		logger.Warningf("query_data searchSource:%s to string error:%v", searchSourceString, err)
	}

	jsonSearchSource, err := json.Marshal(searchSourceString)
	if err != nil {
		logger.Warningf("query_data searchSource:%s to json error:%v", searchSourceString, err)
	}

	result, err := search(ctx, indexArr, searchSource, param.Timeout, param.MaxShard)
	if err != nil {
		logger.Warningf("query_data searchSource:%s query_data error:%v", searchSourceString, err)
		return nil, err
	}

	logger.Debugf("query_data searchSource:%s resp:%s", string(jsonSearchSource), string(result.Aggregations["ts"]))

	js, err := simplejson.NewJson(result.Aggregations["ts"])
	if err != nil {
		return nil, err
	}

	bucketsData, err := js.Get("buckets").Array()
	if err != nil {
		return nil, err
	}

	var keys []string
	for i := len(groupBys) - 1; i >= 0; i-- {
		keys = append(keys, groupBys[i].Field)
	}

	if param.MetricAggr.Func != "count" {
		keys = append(keys, field)
	}

	metrics := &MetricPtr{Data: make(map[string][][]float64)}

	GetBuckts("", keys, bucketsData, metrics, "", 0, param.MetricAggr.Func)

	return TransferData(fmt.Sprintf("%s_%s", field, param.MetricAggr.Func), param.Ref, metrics.Data), nil
}

func HitFilter(typ string) bool {
	switch typ {
	case "keyword", "date", "long", "integer", "short", "byte", "double", "float", "half_float", "scaled_float", "unsigned_long":
		return false
	default:
		return true
	}
}

func QueryLog(ctx context.Context, queryParam interface{}, timeout int64, version string, maxShard int, search SearchFunc) ([]interface{}, int64, error) {
	param := new(Query)
	if err := mapstructure.Decode(queryParam, param); err != nil {
		return nil, 0, err
	}

	if param.Timeout == 0 {
		param.Timeout = int(timeout)
	}

	indexArr := strings.Split(param.Index, ",")

	now := time.Now().Unix()
	var start, end int64
	if param.End != 0 && param.Start != 0 {
		end = param.End - param.End%param.Interval
		start = param.Start - param.Start%param.Interval
	} else {
		end = now
		start = end - param.Interval
	}

	q := elastic.NewRangeQuery(param.DateField)
	q.Gte(time.Unix(start, 0).UnixMilli())
	q.Lte(time.Unix(end, 0).UnixMilli())
	q.Format("epoch_millis")

	queryString := GetQueryString(param.Filter, q)

	if param.Limit <= 0 {
		param.Limit = 10
	}

	if param.MaxShard < 1 {
		param.MaxShard = maxShard
	}

	source := elastic.NewSearchSource().
		TrackTotalHits(true).
		Query(queryString).
		From(param.P).
		Size(param.Limit).
		Sort(param.DateField, param.Ascending)

	result, err := search(ctx, indexArr, source, param.Timeout, param.MaxShard)
	if err != nil {
		logger.Warningf("query data error:%v", err)
		return nil, 0, err
	}

	total := result.TotalHits()

	var ret []interface{}

	b, _ := json.Marshal(source)
	logger.Debugf("query data result query source:%s len:%d total:%d", string(b), len(result.Hits.Hits), total)

	resultBytes, _ := json.Marshal(result)
	logger.Debugf("query data result query source:%s result:%s", string(b), string(resultBytes))

	if strings.HasPrefix(version, "6") {
		for i := 0; i < len(result.Hits.Hits); i++ {
			var x map[string]interface{}
			err := json.Unmarshal(result.Hits.Hits[i].Source, &x)
			if err != nil {
				logger.Warningf("Unmarshal soruce error:%v", err)
				continue
			}

			if result.Hits.Hits[i].Fields == nil {
				result.Hits.Hits[i].Fields = make(map[string]interface{})
			}

			IterGetMap(x, result.Hits.Hits[i].Fields, "")
			ret = append(ret, result.Hits.Hits[i])
		}
	} else {
		for _, hit := range result.Hits.Hits {
			ret = append(ret, hit)
		}
	}

	return ret, total, nil
}
