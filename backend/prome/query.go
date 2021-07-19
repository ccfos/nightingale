package backend

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"
	"github.com/didi/nightingale/v5/vos"
)

const (
	LABEL_IDENT  = "ident"
	LABEL_NAME   = "__name__"
	DEFAULT_QL   = `{__name__=~".*a.*|.*e.*"}`
	DEFAULT_STEP = 15
)

type commonQueryObj struct {
	Idents          []string
	TagPairs        []*vos.TagPair
	Metric          string
	Start           int64
	End             int64
	MetricNameExact bool   // metric_name精确匹配，在查询看图的时候为true
	From            string // 调用的来源
}

// 为查询索引或标签相关的转换，大部分都是正则匹配
func convertToPromql(recv *commonQueryObj) string {

	qlStr := ""
	qlStrFinal := ""
	metricName := ""
	labelIdent := ""
	labelStrSlice := make([]string, 0)
	// 匹配metric_name  __name__=~"xx.*"
	if recv.Metric != "" {
		if recv.MetricNameExact {
			metricName = fmt.Sprintf(`__name__="%s"`, recv.Metric)
		} else {
			metricName = fmt.Sprintf(`__name__=~".*%s.*"`, recv.Metric)
		}

		labelStrSlice = append(labelStrSlice, metricName)

	}
	// 匹配ident=~"k1|k2"
	labelIdent = strings.Join(recv.Idents, "|")
	if labelIdent != "" {
		labelStrSlice = append(labelStrSlice, fmt.Sprintf(`ident=~"%s"`, labelIdent))
	}
	// 匹配标签
	labelM := make(map[string]string)
	for _, i := range recv.TagPairs {
		if i.Key == "" {
			continue
		}
		lastStr, _ := labelM[i.Key]

		lastStr += fmt.Sprintf(`.*%s.*|`, i.Value)
		labelM[i.Key] = lastStr
	}
	for k, v := range labelM {
		thisLabel := strings.TrimRight(v, "|")
		labelStrSlice = append(labelStrSlice, fmt.Sprintf(`%s=~"%s"`, k, thisLabel))

	}

	qlStr = strings.Join(labelStrSlice, ",")
	qlStrFinal = fmt.Sprintf(`{%s}`, qlStr)
	logger.Debugf("[convertToPromql][type=queryLabel][recv:%+v][qlStrFinal:%s]", recv, qlStrFinal)

	return qlStrFinal
}

// 查询数据的转换，metrics_name和标签都是精确匹配
func convertToPromqlForQueryData(recv *commonQueryObj) string {

	qlStr := ""
	qlStrFinal := ""
	metricName := ""
	labelIdent := ""
	labelStrSlice := make([]string, 0)
	// 匹配metric_name  __name__=~"xx.*"
	if recv.Metric != "" {
		metricName = fmt.Sprintf(`__name__="%s"`, recv.Metric)

		labelStrSlice = append(labelStrSlice, metricName)

	}
	// 匹配ident=~"k1|k2"
	labelIdent = strings.Join(recv.Idents, "|")
	if labelIdent != "" {
		labelStrSlice = append(labelStrSlice, fmt.Sprintf(`ident=~"%s"`, labelIdent))
	}
	// 匹配标签
	labelM := make(map[string]string)
	for _, i := range recv.TagPairs {
		if i.Key == "" {
			continue
		}
		lastStr, _ := labelM[i.Key]

		lastStr += fmt.Sprintf(`%s|`, i.Value)
		labelM[i.Key] = lastStr
	}
	for k, v := range labelM {
		thisLabel := strings.TrimRight(v, "|")
		labelStrSlice = append(labelStrSlice, fmt.Sprintf(`%s=~"%s"`, k, thisLabel))

	}

	qlStr = strings.Join(labelStrSlice, ",")
	qlStrFinal = fmt.Sprintf(`{%s}`, qlStr)
	logger.Debugf("[convertToPromql][type=queryData][recv:%+v][qlStrFinal:%s]", recv, qlStrFinal)

	return qlStrFinal
}

func parseMatchersParam(matchers []string) ([][]*labels.Matcher, error) {
	var matcherSets [][]*labels.Matcher
	for _, s := range matchers {
		matchers, err := parser.ParseMetricSelector(s)
		if err != nil {
			return nil, err
		}
		matcherSets = append(matcherSets, matchers)
	}

OUTER:
	for _, ms := range matcherSets {
		for _, lm := range ms {
			if lm != nil && !lm.Matches("") {
				continue OUTER
			}
		}
		return nil, errors.New("match[] must contain at least one non-empty matcher")
	}
	return matcherSets, nil
}

func (pd *PromeDataSource) QueryData(inputs vos.DataQueryParam) []*vos.DataQueryResp {

	respD := make([]*vos.DataQueryResp, 0)
	for _, input := range inputs.Params {
		var qlStrFinal string

		if input.PromeQl != "" {
			qlStrFinal = input.PromeQl
		} else {
			if len(input.Idents) == 0 {
				for i := range input.TagPairs {
					if input.TagPairs[i].Key == "ident" {
						input.Idents = append(input.Idents, input.TagPairs[i].Value)
					}
				}
			}

			if len(input.Idents) == 0 && input.ClasspathId != 0 {
				if input.ClasspathPrefix == 0 {
					classpathAndRes, exists := cache.ClasspathRes.Get(input.ClasspathId)
					if exists {
						input.Idents = classpathAndRes.Res
					}
				} else {
					classpath, err := models.ClasspathGet("id=?", input.ClasspathId)
					if err != nil {
						continue
					}
					cps, _ := models.ClasspathGetsByPrefix(classpath.Path)
					for _, classpath := range cps {
						classpathAndRes, exists := cache.ClasspathRes.Get(classpath.Id)
						if exists {
							idents := classpathAndRes.Res
							input.Idents = append(input.Idents, idents...)
						}
					}
				}
			}

			cj := &commonQueryObj{
				Idents:          input.Idents,
				TagPairs:        input.TagPairs,
				Metric:          input.Metric,
				Start:           inputs.Start,
				End:             inputs.End,
				MetricNameExact: true,
			}
			qlStrFinal = convertToPromqlForQueryData(cj)

		}

		logger.Debugf("[input:%+v][qlStrFinal:%s]\n", input, qlStrFinal)
		// 转化为utc时间
		startT := tsToUtcTs(inputs.Start)
		endT := tsToUtcTs(inputs.End)

		// TODO 前端传入分辨率还是后端计算，grafana和prometheus ui都是前端传入
		delta := (inputs.End - inputs.Start) / 3600
		if delta <= 0 {
			delta = 1
		}
		resolution := time.Second * time.Duration(delta*DEFAULT_STEP)

		q, err := pd.QueryEngine.NewRangeQuery(pd.Queryable, qlStrFinal, startT, endT, resolution)
		if err != nil {
			logger.Errorf("[prome_query_error][QueryData_error_may_be_parse_ql_error][args:%+v][err:%+v]", input, err)
			continue
		}
		ctx, _ := context.WithTimeout(context.Background(), time.Second*30)
		res := q.Exec(ctx)
		// TODO 将err返回给前端
		if res.Err != nil {
			logger.Errorf("[prome_query_error][rangeQuery_exec_error][args:%+v][err:%+v]", input, res.Err)
			q.Close()
			continue
		}
		mat, ok := res.Value.(promql.Matrix)
		if !ok {
			logger.Errorf("[promql.Engine.exec: invalid expression type %q]", res.Value.Type())
			q.Close()
			continue
		}
		if res.Err != nil {
			logger.Errorf("[prome_query_error][res.Matrix_error][args:%+v][err:%+v]", input, res.Err)
			q.Close()
			continue
		}
		for index, m := range mat {
			if inputs.Limit > 0 && index+1 > inputs.Limit {
				continue
			}
			tagStr := ""
			oneResp := &vos.DataQueryResp{}

			ident := m.Metric.Get(LABEL_IDENT)
			name := m.Metric.Get(LABEL_NAME)
			oneResp.Metric = name
			oneResp.Ident = ident
			// TODO 去掉point num
			pNum := len(m.Points)
			for _, p := range m.Points {
				tmpP := &vos.Point{
					Timestamp: p.T,
					Value:     vos.JsonFloat(p.V),
				}
				oneResp.Values = append(oneResp.Values, tmpP)
			}
			for _, x := range m.Metric {
				if x.Name == LABEL_NAME {
					continue
				}
				tagStr += fmt.Sprintf("%s=%s,", x.Name, x.Value)
			}
			tagStr = strings.TrimRight(tagStr, ",")
			oneResp.Tags = tagStr
			oneResp.Resolution = delta * DEFAULT_STEP
			oneResp.PNum = pNum
			respD = append(respD, oneResp)

		}
		q.Close()

	}
	return respD
}

func tsToUtcTs(s int64) time.Time {
	return time.Unix(s, 0).UTC()
}
func timeParse(ts int64) time.Time {
	t := float64(ts)
	s, ns := math.Modf(t)
	ns = math.Round(ns*1000) / 1000
	return time.Unix(int64(s), int64(ns*float64(time.Second))).UTC()
}

func millisecondTs(t time.Time) int64 {
	return t.Unix()*1000 + int64(t.Nanosecond())/int64(time.Millisecond)
}
func tsToStr(timestamp int64) string {
	timeNow := time.Unix(timestamp, 0)
	return timeNow.Format("2006-01-02 15:04:05")
}

func (pd *PromeDataSource) CommonQuerySeries(cj *commonQueryObj) storage.SeriesSet {
	qlStrFinal := convertToPromql(cj)

	if qlStrFinal == "{}" {
		qlStrFinal = DEFAULT_QL
		reqMinute := (cj.End - cj.Start) / 60
		// 如果前端啥都没传，要限制下查询series的时间范围，防止高基础查询
		if reqMinute > pd.Section.MaxFetchAllSeriesLimitMinute {
			// 时间超长，用配置文件中的限制一下
			now := time.Now().Unix()
			cj.End = now
			cj.Start = now - pd.Section.MaxFetchAllSeriesLimitMinute*60
			logger.Debugf("[CommonQuerySeries.FetchAllSeries.LimitQueryTimeRange][start:%v][end:%v]", cj.Start, cj.End)
		}
	}

	matcherSets, err := parseMatchersParam([]string{qlStrFinal})
	if err != nil {
		logger.Errorf("[prome_query_error][parse_label_match_error][err:%+v]", err)
		return nil
	}
	now := time.Now().Unix()
	if cj.Start == 0 {
		cj.Start = now - 60*pd.Section.MaxFetchAllSeriesLimitMinute
	}
	if cj.End == 0 {
		cj.End = now
	}

	startT := millisecondTs(timeParse(cj.Start))
	endT := millisecondTs(timeParse(cj.End))

	ctx, _ := context.WithTimeout(context.Background(), time.Second*30)
	q, err := pd.Queryable.Querier(ctx, startT, endT)
	if err != nil {

		logger.Errorf("[prome_query_error][get_querier_errro]")
		return nil
	}
	logger.Debugf("[CommonQuerySeries.Result][from:%s][cj.start_ts:%+v cj.start_str:%+v SelectHints.startT:%+v][cj.end_ts:%+v cj.end_str:%+v  SelectHints.endT:%+v][qlStrFinal:%s][cj:%+v]",
		cj.From,
		cj.Start,
		tsToStr(cj.Start),
		startT,
		cj.End,
		tsToStr(cj.End),
		endT,
		qlStrFinal,
		cj,
	)

	defer q.Close()

	hints := &storage.SelectHints{
		Start: startT,
		End:   endT,
		Func:  "series", // There is no series function, this token is used for lookups that don't need samples.
	}

	// Get all series which match matchers.
	s := q.Select(true, hints, matcherSets[0]...)
	return s

}

// 全部转化为 {__name__="a",label_a!="b",label_b=~"d|c",label_c!~"d"}
// 对应prometheus 中的 /api/v1/labels
// TODO 等待prometheus官方对 remote_read label_values 的支持
// Implement: https://github.com/prometheus/prometheus/issues/3351
func (pd *PromeDataSource) QueryTagKeys(recv vos.CommonTagQueryParam) *vos.TagKeyQueryResp {
	// TODO 完成标签匹配模式
	respD := &vos.TagKeyQueryResp{
		Keys: make([]string, 0),
	}

	labelNamesSet := make(map[string]struct{})
	if len(recv.Params) == 0 {
		recv.Params = append(recv.Params, vos.TagPairQueryParamOne{
			Idents: []string{},
			Metric: "",
		})
	}

	for _, x := range recv.Params {
		cj := &commonQueryObj{
			Idents:   x.Idents,
			TagPairs: recv.TagPairs,
			Metric:   x.Metric,
			Start:    recv.Start,
			End:      recv.End,
			From:     "QueryTagKeys",
		}

		s := pd.CommonQuerySeries(cj)
		if s.Warnings() != nil {
			logger.Warningf("[prome_query_error][series_set_iter_error][warning:%+v]", s.Warnings())

		}

		if err := s.Err(); err != nil {
			logger.Errorf("[prome_query_error][series_set_iter_error][err:%+v]", err)
			continue
		}
		for s.Next() {
			series := s.At()
			for _, lb := range series.Labels() {
				if lb.Name == LABEL_NAME {
					continue

				}
				if recv.TagKey != "" {
					if !strings.Contains(lb.Name, recv.TagKey) {
						continue
					}
				}
				labelNamesSet[lb.Name] = struct{}{}
			}
		}

	}
	names := make([]string, 0)
	for key := range labelNamesSet {

		names = append(names, key)
	}
	sort.Strings(names)
	// 因为map中的key是无序的，必须这样才能稳定输出
	if recv.Limit > 0 && len(names) > recv.Limit {
		names = names[:recv.Limit]
	}

	respD.Keys = names
	return respD

}

// 对应prometheus 中的 /api/v1/label/<label_name>/values
func (pd *PromeDataSource) QueryTagValues(recv vos.CommonTagQueryParam) *vos.TagValueQueryResp {
	labelValuesSet := make(map[string]struct{})

	if len(recv.Params) == 0 {
		recv.Params = append(recv.Params, vos.TagPairQueryParamOne{
			Idents: []string{},
			Metric: "",
		})
	}

	for _, x := range recv.Params {
		cj := &commonQueryObj{
			Idents:   x.Idents,
			Metric:   x.Metric,
			TagPairs: recv.TagPairs,
			Start:    recv.Start,
			End:      recv.End,
			From:     "QueryTagValues",
		}

		s := pd.CommonQuerySeries(cj)
		if s.Warnings() != nil {
			logger.Warningf("[prome_query_error][series_set_iter_error][warning:%+v]", s.Warnings())

		}

		if err := s.Err(); err != nil {
			logger.Errorf("[prome_query_error][series_set_iter_error][err:%+v]", err)
			continue
		}

		for s.Next() {
			series := s.At()
			for _, lb := range series.Labels() {
				if lb.Name == recv.TagKey {
					if recv.TagValue != "" {
						if !strings.Contains(lb.Value, recv.TagValue) {
							continue
						}
					}

					labelValuesSet[lb.Value] = struct{}{}
				}
			}
		}
	}
	vals := make([]string, 0)
	for val := range labelValuesSet {

		vals = append(vals, val)
	}
	sort.Strings(vals)
	if recv.Limit > 0 && len(vals) > recv.Limit {
		vals = vals[:recv.Limit]
	}
	respD := &vos.TagValueQueryResp{}
	respD.Values = vals
	return respD

}

// 对应prometheus 中的 /api/v1/label/<label_name>/values label_name == __name__
func (pd *PromeDataSource) QueryMetrics(recv vos.MetricQueryParam) *vos.MetricQueryResp {
	cj := &commonQueryObj{
		Idents:   recv.Idents,
		Metric:   recv.Metric,
		TagPairs: recv.TagPairs,
		Start:    recv.Start,
		End:      recv.End,
		From:     "QueryMetrics",
	}

	respD := &vos.MetricQueryResp{}
	respD.Metrics = make([]string, 0)
	s := pd.CommonQuerySeries(cj)
	for _, x := range s.Warnings() {
		logger.Warningf("[prome_query_error][series_set_iter_error][warning:%+v]\n", x.Error())

	}

	if err := s.Err(); err != nil {
		logger.Errorf("[prome_query_error][series_set_iter_error][err:%+v]", err)
		return respD
	}

	var sets []storage.SeriesSet
	sets = append(sets, s)
	set := storage.NewMergeSeriesSet(sets, storage.ChainedSeriesMerge)
	labelValuesSet := make(map[string]struct{})
	//for s.Next() {
	for set.Next() {
		series := set.At()
		for _, lb := range series.Labels() {
			if lb.Name == LABEL_NAME {
				labelValuesSet[lb.Value] = struct{}{}
			}
		}
	}
	vals := make([]string, 0)
	for val := range labelValuesSet {
		vals = append(vals, val)
	}

	sort.Strings(vals)

	if recv.Limit > 0 && len(vals) > recv.Limit {
		vals = vals[:recv.Limit]
	}
	respD.Metrics = vals
	return respD
}

// 对应prometheus 中的 /api/v1/series
func (pd *PromeDataSource) QueryTagPairs(recv vos.CommonTagQueryParam) *vos.TagPairQueryResp {
	respD := &vos.TagPairQueryResp{
		TagPairs: make([]string, 0),
		Idents:   make([]string, 0),
	}
	tps := make(map[string]struct{})
	if len(recv.Params) == 0 {
		recv.Params = append(recv.Params, vos.TagPairQueryParamOne{
			Idents: []string{},
			Metric: "",
		})
	}
	for _, x := range recv.Params {
		cj := &commonQueryObj{
			Idents:   x.Idents,
			TagPairs: recv.TagPairs,
			Metric:   x.Metric,
			Start:    recv.Start,
			End:      recv.End,
			From:     "QueryTagPairs",
		}

		s := pd.CommonQuerySeries(cj)
		if s.Warnings() != nil {
			logger.Warningf("[prome_query_error][series_set_iter_error][warning:%+v]", s.Warnings())

		}

		if err := s.Err(); err != nil {
			logger.Errorf("[prome_query_error][series_set_iter_error][err:%+v]", err)
			continue
		}

		var sets []storage.SeriesSet
		sets = append(sets, s)
		set := storage.NewMergeSeriesSet(sets, storage.ChainedSeriesMerge)

		labelIdents := make([]string, 0)
		for set.Next() {
			series := s.At()
			labelsS := series.Labels()
			for _, i := range labelsS {

				if i.Name == LABEL_NAME {
					continue
				}
				if i.Name == LABEL_IDENT {
					labelIdents = append(labelIdents, i.Value)
				}
				tps[fmt.Sprintf("%s=%s", i.Name, i.Value)] = struct{}{}
			}

		}

	}

	newTags := make([]string, 0)
	for k := range tps {

		newTags = append(newTags, k)
	}

	sort.Strings(newTags)
	if recv.Limit > 0 && len(newTags) > recv.Limit {
		newTags = newTags[:recv.Limit]
	}

	respD.TagPairs = newTags
	return respD
}

func (pd *PromeDataSource) QueryDataInstant(ql string) []*vos.DataQueryInstanceResp {
	respD := make([]*vos.DataQueryInstanceResp, 0)
	pv := pd.QueryVector(ql)
	if pv == nil {

		return respD
	}

	for _, s := range pv {
		metricOne := make(map[string]interface{})
		valueOne := make([]float64, 0)

		for _, l := range s.Metric {
			if l.Name == LABEL_NAME {
				continue
			}
			metricOne[l.Name] = l.Value
		}
		// 毫秒时间时间戳转 秒时间戳
		valueOne = append(valueOne, float64(s.Point.T)/1e3)
		valueOne = append(valueOne, s.Point.V)
		respD = append(respD, &vos.DataQueryInstanceResp{
			Metric: metricOne,
			Value:  valueOne,
		})

	}
	return respD
}

func (pd *PromeDataSource) QueryVector(ql string) promql.Vector {
	t := time.Now()
	q, err := pd.QueryEngine.NewInstantQuery(pd.Queryable, ql, t)
	if err != nil {
		logger.Errorf("[prome_query_error][new_insQuery_error][err:%+v][ql:%+v]", err, ql)
		return nil
	}
	ctx := context.Background()
	res := q.Exec(ctx)
	if res.Err != nil {
		logger.Errorf("[prome_query_error][insQuery_exec_error][err:%+v][ql:%+v]", err, ql)
		return nil
	}
	defer q.Close()
	switch v := res.Value.(type) {
	case promql.Vector:
		return v
	case promql.Scalar:
		return promql.Vector{promql.Sample{
			Point:  promql.Point(v),
			Metric: labels.Labels{},
		}}
	default:
		logger.Errorf("[prome_query_error][insQuery_res_error rule result is not a vector or scalar][err:%+v][ql:%+v]", err, ql)
		return nil
	}

}
