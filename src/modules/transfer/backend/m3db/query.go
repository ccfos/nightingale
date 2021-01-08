package m3db

import (
	"strings"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/m3db/m3/src/dbnode/storage/index"
	"github.com/m3db/m3/src/m3ninx/idx"
)

// QueryData
func (cfg M3dbSection) queryDataOptions(inputs []dataobj.QueryData) (index.Query, index.QueryOptions) {
	q := []idx.Query{}

	for _, input := range inputs {
		q1 := endpointsQuery(input.Nids, input.Endpoints)
		q2 := counterQuery(input.Counters)
		q = append(q, idx.NewConjunctionQuery(q1, q2))
	}

	return index.Query{idx.NewDisjunctionQuery(q...)},
		index.QueryOptions{
			StartInclusive: time.Unix(inputs[0].Start, 0),
			EndExclusive:   time.Unix(inputs[0].End+1, 0),
			SeriesLimit:    cfg.SeriesLimit,
			DocsLimit:      cfg.DocsLimit,
		}
}

// QueryDataForUI
// metric && (endpoints[0] || endporint[1] ...) && (tags[0] || tags[1] ...)
func (cfg M3dbSection) queryDataUIOptions(input dataobj.QueryDataForUI) (index.Query, index.QueryOptions) {
	q1 := idx.NewTermQuery([]byte(METRIC_NAME), []byte(input.Metric))
	q2 := endpointsQuery(input.Nids, input.Endpoints)
	q3 := metricTagsQuery(input.Tags)

	return index.Query{idx.NewConjunctionQuery(q1, q2, q3)},
		index.QueryOptions{
			StartInclusive: time.Unix(input.Start, 0),
			EndExclusive:   time.Unix(input.End+1, 0),
			SeriesLimit:    cfg.SeriesLimit,
			DocsLimit:      cfg.DocsLimit,
		}
}

func metricsQuery(metrics []string) idx.Query {
	q := []idx.Query{}
	for _, v := range metrics {
		q = append(q, idx.NewTermQuery([]byte(METRIC_NAME), []byte(v)))
	}
	return idx.NewDisjunctionQuery(q...)
}

func metricQuery(metric string) idx.Query {
	return idx.NewTermQuery([]byte(METRIC_NAME), []byte(metric))
}

func endpointsQuery(nids, endpoints []string) idx.Query {
	if len(nids) > 0 {
		q := []idx.Query{}
		for _, v := range nids {
			q = append(q, idx.NewTermQuery([]byte(NID_NAME), []byte(v)))
		}
		if len(q) == 1 {
			return q[0]
		}
		return idx.NewDisjunctionQuery(q...)
	}

	if len(endpoints) > 0 {
		q := []idx.Query{}
		for _, v := range endpoints {
			q = append(q, idx.NewTermQuery([]byte(ENDPOINT_NAME), []byte(v)))
		}
		if len(q) == 1 {
			return q[0]
		}
		return idx.NewDisjunctionQuery(q...)
	}

	return idx.NewAllQuery()
}

func counterQuery(counters []string) idx.Query {
	q := []idx.Query{}

	for _, v := range counters {
		items := strings.SplitN(v, "/", 2)

		var metric, tag string
		if len(items) == 2 {
			metric, tag = items[0], items[1]
		} else if len(items) == 1 && len(items[0]) > 0 {
			metric = items[0]
		} else {
			continue
		}

		tagMap := dataobj.DictedTagstring(tag)

		q2 := []idx.Query{}
		q2 = append(q2, idx.NewTermQuery([]byte(METRIC_NAME), []byte(metric)))

		for k, v := range tagMap {
			q2 = append(q2, idx.NewTermQuery([]byte(k), []byte(v)))
		}
		q = append(q, idx.NewConjunctionQuery(q2...))
	}

	if len(q) > 0 {
		return idx.NewDisjunctionQuery(q...)
	}

	return idx.NewAllQuery()
}

// (tags[0] || tags[2] || ...)
func metricTagsQuery(tags []string) idx.Query {
	if len(tags) == 0 {
		return idx.NewAllQuery()
	}

	q := []idx.Query{}
	for _, v := range tags {
		q1 := []idx.Query{}
		tagMap := dataobj.DictedTagstring(v)

		for k, v := range tagMap {
			q1 = append(q1, idx.NewTermQuery([]byte(k), []byte(v)))
		}
		q = append(q, idx.NewConjunctionQuery(q1...))
	}

	return idx.NewDisjunctionQuery(q...)
}

// QueryMetrics
// (endpoint[0] || endpoint[1] ... )
func (cfg M3dbSection) queryMetricsOptions(input dataobj.EndpointsRecv) (index.Query, index.AggregationOptions) {
	nameByte := []byte(METRIC_NAME)
	return index.Query{idx.NewConjunctionQuery(
			endpointsQuery(input.Nids, input.Endpoints),
			idx.NewFieldQuery(nameByte),
		)},
		index.AggregationOptions{
			QueryOptions: index.QueryOptions{
				StartInclusive: indexStartTime(),
				EndExclusive:   time.Now(),
				SeriesLimit:    cfg.SeriesLimit,
				DocsLimit:      cfg.DocsLimit,
			},
			FieldFilter: [][]byte{nameByte},
			Type:        index.AggregateTagNamesAndValues,
		}
}

// QueryTagPairs
// (endpoint[0] || endpoint[1]...) && (metrics[0] || metrics[1] ... )
func (cfg M3dbSection) queryTagPairsOptions(input dataobj.EndpointMetricRecv) (index.Query, index.AggregationOptions) {
	q1 := endpointsQuery(input.Nids, input.Endpoints)
	q2 := metricsQuery(input.Metrics)

	return index.Query{idx.NewConjunctionQuery(q1, q2)},
		index.AggregationOptions{
			QueryOptions: index.QueryOptions{
				StartInclusive: indexStartTime(),
				EndExclusive:   time.Now(),
				SeriesLimit:    cfg.SeriesLimit,
				DocsLimit:      cfg.DocsLimit,
			},
			FieldFilter: index.AggregateFieldFilter(nil),
			Type:        index.AggregateTagNamesAndValues,
		}
}

// QueryIndexByClude: || (&& (|| endpoints...) (metric) (|| include...) (&& exclude..))
func (cfg M3dbSection) queryIndexByCludeOptions(input dataobj.CludeRecv) (index.Query, index.QueryOptions) {
	query := index.Query{}
	q := []idx.Query{}

	if len(input.Endpoints) > 0 || len(input.Nids) > 0 {
		q = append(q, endpointsQuery(input.Nids, input.Endpoints))
	}
	if input.Metric != "" {
		q = append(q, metricQuery(input.Metric))
	}
	if len(input.Include) > 0 {
		q = append(q, includeTagsQuery(input.Include))
	}
	if len(input.Exclude) > 0 {
		q = append(q, excludeTagsQuery(input.Exclude))
	}

	if len(q) == 0 {
		query = index.Query{idx.NewAllQuery()}
	} else if len(q) == 1 {
		query = index.Query{q[0]}
	} else {
		query = index.Query{idx.NewConjunctionQuery(q...)}
	}

	return query, index.QueryOptions{
		StartInclusive: indexStartTime(),
		EndExclusive:   time.Now(),
		SeriesLimit:    cfg.SeriesLimit,
		DocsLimit:      cfg.DocsLimit,
	}

}

// QueryIndexByFullTags: && (|| endpoints) (metric) (&& tagkv)
func (cfg M3dbSection) queryIndexByFullTagsOptions(input dataobj.IndexByFullTagsRecv) (index.Query, index.QueryOptions) {
	query := index.Query{}
	q := []idx.Query{}

	if len(input.Endpoints) > 0 || len(input.Nids) > 0 {
		q = append(q, endpointsQuery(input.Nids, input.Endpoints))
	}
	if input.Metric != "" {
		q = append(q, metricQuery(input.Metric))
	}
	if len(input.Tagkv) > 0 {
		q = append(q, includeTagsQuery2(input.Tagkv))
	}

	if len(q) == 0 {
		query = index.Query{idx.NewAllQuery()}
	} else {
		query = index.Query{idx.NewConjunctionQuery(q...)}
	}

	return query, index.QueryOptions{
		StartInclusive: input.StartInclusive,
		EndExclusive:   input.EndExclusive,
		SeriesLimit:    cfg.SeriesLimit,
		DocsLimit:      cfg.DocsLimit,
	}
}

// && ((|| values...))...
func includeTagsQuery(in []*dataobj.TagPair) idx.Query {
	q := []idx.Query{}
	for _, kvs := range in {
		q1 := []idx.Query{}
		for _, v := range kvs.Values {
			q1 = append(q1, idx.NewTermQuery([]byte(kvs.Key), []byte(v)))
		}
		if len(q1) > 0 {
			q = append(q, idx.NewDisjunctionQuery(q1...))
		}
	}

	if len(q) == 0 {
		return idx.NewAllQuery()
	}

	return idx.NewConjunctionQuery(q...)
}

func includeTagsQuery2(in []dataobj.TagPair) idx.Query {
	q := []idx.Query{}
	for _, kvs := range in {
		q1 := []idx.Query{}
		for _, v := range kvs.Values {
			q1 = append(q1, idx.NewTermQuery([]byte(kvs.Key), []byte(v)))
		}
		if len(q1) > 0 {
			q = append(q, idx.NewDisjunctionQuery(q1...))
		}
	}

	if len(q) == 0 {
		return idx.NewAllQuery()
	}

	return idx.NewConjunctionQuery(q...)
}

// && (&& !values...)
func excludeTagsQuery(in []*dataobj.TagPair) idx.Query {
	q := []idx.Query{}
	for _, kvs := range in {
		for _, v := range kvs.Values {
			q = append(q, idx.NewNegationQuery(idx.NewTermQuery([]byte(kvs.Key), []byte(v))))
		}
	}

	if len(q) == 0 {
		return idx.NewAllQuery()
	}

	return idx.NewConjunctionQuery(q...)
}
