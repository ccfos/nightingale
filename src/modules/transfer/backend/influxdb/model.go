package influxdb

import (
	"fmt"
	"strings"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"

	"github.com/toolkits/pkg/logger"
)

type ShowSeries struct {
	Database  string
	Metric    string
	Endpoints []string
	Include   []*dataobj.TagPair
	Exclude   []*dataobj.TagPair
	Start     int64
	End       int64

	RawQuery string
}

func (query *ShowSeries) renderShow() {
	query.RawQuery = fmt.Sprintf("SHOW SERIES ON \"%s\" FROM \"%s\"", query.Database,
		query.Metric)
}

func (query *ShowSeries) renderEndpoints() {
	if len(query.Endpoints) > 0 {
		// endpoints
		endpointPart := "("
		for _, endpoint := range query.Endpoints {
			endpointPart += fmt.Sprintf(" \"endpoint\"='%s' OR", endpoint)
		}
		endpointPart = endpointPart[:len(endpointPart)-len("OR")]
		endpointPart += ")"
		query.RawQuery = fmt.Sprintf("%s WHERE %s", query.RawQuery, endpointPart)
	}
}

func (query *ShowSeries) renderInclude() {
	if len(query.Include) > 0 {
		// include
		if len(query.Include) == 1 && query.Include[0] == nil {
			return
		}
		includePart := "("
		for _, include := range query.Include {
			for _, value := range include.Values {
				includePart += fmt.Sprintf(" \"%s\"='%s' OR", include.Key, value)
			}
		}
		includePart = includePart[:len(includePart)-len("AND")]
		includePart += ")"
		if !strings.Contains(query.RawQuery, "WHERE") {
			query.RawQuery = fmt.Sprintf(" %s WHERE %s", query.RawQuery, includePart)
		} else {
			query.RawQuery = fmt.Sprintf(" %s AND %s", query.RawQuery, includePart)
		}
	}
}

func (query *ShowSeries) renderExclude() {
	if len(query.Exclude) > 0 {
		// exclude
		if len(query.Exclude) == 1 && query.Exclude[0] == nil {
			return
		}
		excludePart := "("
		for _, exclude := range query.Exclude {
			for _, value := range exclude.Values {
				excludePart += fmt.Sprintf(" \"%s\"!='%s' AND", exclude.Key, value)
			}
		}
		excludePart = excludePart[:len(excludePart)-len("AND")]
		excludePart += ")"
		if !strings.Contains(query.RawQuery, "WHERE") {
			query.RawQuery = fmt.Sprintf(" %s WHERE %s", query.RawQuery, excludePart)
		} else {
			query.RawQuery = fmt.Sprintf(" %s AND %s", query.RawQuery, excludePart)
		}
	}
}

func (query *ShowSeries) renderTimeRange() {
	// time
	if strings.Contains(query.RawQuery, "WHERE") {
		query.RawQuery = fmt.Sprintf("%s AND time >= %d AND time <= %d", query.RawQuery,
			time.Duration(query.Start)*time.Second,
			time.Duration(query.End)*time.Second)
	} else {
		query.RawQuery = fmt.Sprintf("%s WHERE time >= %d AND time <= %d", query.RawQuery,
			time.Duration(query.Start)*time.Second,
			time.Duration(query.End)*time.Second)
	}
}

type QueryData struct {
	Start     int64
	End       int64
	Metric    string
	Endpoints []string
	Tags      []string
	Step      int
	DsType    string
	GroupKey  []string //聚合维度
	AggrFunc  string   //聚合计算

	RawQuery string
}

func (query *QueryData) renderSelect() {
	// select
	if query.AggrFunc != "" && len(query.GroupKey) > 0 {
		query.RawQuery = ""
	} else {
		query.RawQuery = fmt.Sprintf("SELECT \"value\" FROM \"%s\"", query.Metric)
	}
}

func (query *QueryData) renderEndpoints() {
	// where endpoint
	if len(query.Endpoints) > 0 {
		endpointPart := "("
		for _, endpoint := range query.Endpoints {
			endpointPart += fmt.Sprintf(" \"endpoint\"='%s' OR", endpoint)
		}
		endpointPart = endpointPart[:len(endpointPart)-len("OR")]
		endpointPart += ")"
		query.RawQuery = fmt.Sprintf("%s WHERE %s", query.RawQuery, endpointPart)
	}
}

func (query *QueryData) renderTags() {
	// where tags
	if len(query.Tags) > 0 {
		s := strings.Join(query.Tags, ",")
		tags, err := dataobj.SplitTagsString(s)
		if err != nil {
			logger.Warningf("split tags error, %+v", err)
			return
		}

		tagPart := "("
		for tagK, tagV := range tags {
			tagPart += fmt.Sprintf(" \"%s\"='%s' AND", tagK, tagV)
		}
		tagPart = tagPart[:len(tagPart)-len("AND")]
		tagPart += ")"

		if strings.Contains(query.RawQuery, "WHERE") {
			query.RawQuery = fmt.Sprintf("%s AND %s", query.RawQuery, tagPart)
		} else {
			query.RawQuery = fmt.Sprintf("%s WHERE %s", query.RawQuery, tagPart)
		}
	}
}

func (query *QueryData) renderTimeRange() {
	// time
	if strings.Contains(query.RawQuery, "WHERE") {
		query.RawQuery = fmt.Sprintf("%s AND time >= %d AND time <= %d", query.RawQuery,
			time.Duration(query.Start)*time.Second,
			time.Duration(query.End)*time.Second)
	} else {
		query.RawQuery = fmt.Sprintf("%s WHERE time >= %d AND time <= %d", query.RawQuery, query.Start, query.End)
	}
}

func (query *QueryData) renderGroupBy() {
	// group by
	if len(query.GroupKey) > 0 {
		groupByPart := strings.Join(query.GroupKey, ",")
		query.RawQuery = fmt.Sprintf("%s GROUP BY %s", query.RawQuery, groupByPart)
	}
}
