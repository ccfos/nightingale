package influxdb

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/influxdata/influxdb/models"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/influxdata/influxdb/client/v2"
	"github.com/toolkits/pkg/logger"
)

// select value from metric where ...
func (influxdb *InfluxdbDataSource) QueryData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse {
	logger.Debugf("query data, inputs: %+v", inputs)

	c, err := NewInfluxdbClient(influxdb.Section)
	defer c.Client.Close()

	if err != nil {
		logger.Errorf("init influxdb client fail: %v", err)
		return nil
	}

	queryResponse := make([]*dataobj.TsdbQueryResponse, 0)
	for _, input := range inputs {
		for _, counter := range input.Counters {
			items := strings.Split(counter, "/")
			metric := items[0]
			var tags = make([]string, 0)
			if len(items) > 1 && len(items[1]) > 0 {
				tags = strings.Split(items[1], ",")
			}
			influxdbQuery := QueryData{
				Start:     input.Start,
				End:       input.End,
				Metric:    metric,
				Endpoints: input.Endpoints,
				Tags:      tags,
				Step:      input.Step,
				DsType:    input.DsType,
			}
			influxdbQuery.renderSelect()
			influxdbQuery.renderEndpoints()
			influxdbQuery.renderTags()
			influxdbQuery.renderTimeRange()
			logger.Debugf("query influxql %s", influxdbQuery.RawQuery)

			query := client.NewQuery(influxdbQuery.RawQuery, c.Database, c.Precision)
			if response, err := c.Client.Query(query); err == nil && response.Error() == nil {
				for _, result := range response.Results {
					for _, series := range result.Series {

						// fixme : influx client get series.Tags is nil
						endpoint := series.Tags["endpoint"]
						delete(series.Tags, endpoint)
						counter, err := dataobj.GetCounter(series.Name, "", series.Tags)
						if err != nil {
							logger.Warningf("get counter error: %+v", err)
							continue
						}
						values := convertValues(series)

						resp := &dataobj.TsdbQueryResponse{
							Start:    influxdbQuery.Start,
							End:      influxdbQuery.End,
							Endpoint: endpoint,
							Counter:  counter,
							DsType:   influxdbQuery.DsType,
							Step:     influxdbQuery.Step,
							Values:   values,
						}
						queryResponse = append(queryResponse, resp)
					}
				}
			}
		}
	}
	return queryResponse
}

// todo : 支持 comparison
// select value from metric where ...
func (influxdb *InfluxdbDataSource) QueryDataForUI(input dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {

	logger.Debugf("query data for ui, input: %+v", input)

	c, err := NewInfluxdbClient(influxdb.Section)
	defer c.Client.Close()

	if err != nil {
		logger.Errorf("init influxdb client fail: %v", err)
		return nil
	}

	influxdbQuery := QueryData{
		Start:     input.Start,
		End:       input.End,
		Metric:    input.Metric,
		Endpoints: input.Endpoints,
		Tags:      input.Tags,
		Step:      input.Step,
		DsType:    input.DsType,
		GroupKey:  input.GroupKey,
		AggrFunc:  input.AggrFunc,
	}
	influxdbQuery.renderSelect()
	influxdbQuery.renderEndpoints()
	influxdbQuery.renderTags()
	influxdbQuery.renderTimeRange()
	influxdbQuery.renderGroupBy()
	logger.Debugf("query influxql %s", influxdbQuery.RawQuery)

	queryResponse := make([]*dataobj.TsdbQueryResponse, 0)
	query := client.NewQuery(influxdbQuery.RawQuery, c.Database, c.Precision)
	if response, err := c.Client.Query(query); err == nil && response.Error() == nil {

		for _, result := range response.Results {
			for _, series := range result.Series {

				// fixme : influx client get series.Tags is nil
				endpoint := series.Tags["endpoint"]
				delete(series.Tags, endpoint)
				counter, err := dataobj.GetCounter(series.Name, "", series.Tags)
				if err != nil {
					logger.Warningf("get counter error: %+v", err)
					continue
				}
				values := convertValues(series)

				resp := &dataobj.TsdbQueryResponse{
					Start:    influxdbQuery.Start,
					End:      influxdbQuery.End,
					Endpoint: endpoint,
					Counter:  counter,
					DsType:   influxdbQuery.DsType,
					Step:     influxdbQuery.Step,
					Values:   values,
				}
				queryResponse = append(queryResponse, resp)
			}
		}
	}
	return queryResponse
}

// show measurements on n9e
func (influxdb *InfluxdbDataSource) QueryMetrics(recv dataobj.EndpointsRecv) *dataobj.MetricResp {
	logger.Debugf("query metric, recv: %+v", recv)

	c, err := NewInfluxdbClient(influxdb.Section)
	defer c.Client.Close()

	if err != nil {
		logger.Errorf("init influxdb client fail: %v", err)
		return nil
	}

	influxql := fmt.Sprintf("SHOW MEASUREMENTS ON \"%s\"", influxdb.Section.Database)
	query := client.NewQuery(influxql, c.Database, c.Precision)
	if response, err := c.Client.Query(query); err == nil && response.Error() == nil {
		resp := &dataobj.MetricResp{
			Metrics: make([]string, 0),
		}
		for _, result := range response.Results {
			for _, series := range result.Series {
				for _, valuePair := range series.Values {
					metric := valuePair[0].(string)
					resp.Metrics = append(resp.Metrics, metric)
				}
			}
		}
		return resp
	}
	return nil
}

// show tag keys / values from metric ...
func (influxdb *InfluxdbDataSource) QueryTagPairs(recv dataobj.EndpointMetricRecv) []dataobj.IndexTagkvResp {
	logger.Debugf("query tag pairs, recv: %+v", recv)

	c, err := NewInfluxdbClient(influxdb.Section)
	defer c.Client.Close()

	if err != nil {
		logger.Errorf("init influxdb client fail: %v", err)
		return nil
	}

	resp := make([]dataobj.IndexTagkvResp, 0)
	for _, metric := range recv.Metrics {
		tagkvResp := dataobj.IndexTagkvResp{
			Endpoints: recv.Endpoints,
			Metric:    metric,
			Tagkv:     make([]*dataobj.TagPair, 0),
		}
		// show tag keys
		keys := showTagKeys(c, metric, influxdb.Section.Database)
		if len(keys) > 0 {
			// show tag values
			tagkvResp.Tagkv = showTagValues(c, keys, metric, influxdb.Section.Database)
		}
		resp = append(resp, tagkvResp)
	}

	return resp
}

// show tag keys on n9e from metric where ...
// (exclude default endpoint tag)
func showTagKeys(c *InfluxClient, metric, database string) []string {
	keys := make([]string, 0)
	influxql := fmt.Sprintf("SHOW TAG KEYS ON \"%s\" FROM \"%s\"", database, metric)
	query := client.NewQuery(influxql, c.Database, c.Precision)
	if response, err := c.Client.Query(query); err == nil && response.Error() == nil {
		for _, result := range response.Results {
			for _, series := range result.Series {
				for _, valuePair := range series.Values {
					tagKey := valuePair[0].(string)
					// 去掉默认tag endpoint
					if tagKey != "endpoint" {
						keys = append(keys, tagKey)
					}
				}
			}
		}
	}
	return keys
}

// show tag values on n9e from metric where ...
func showTagValues(c *InfluxClient, keys []string, metric, database string) []*dataobj.TagPair {
	tagkv := make([]*dataobj.TagPair, 0)
	influxql := fmt.Sprintf("SHOW TAG VALUES ON \"%s\" FROM \"%s\" WITH KEY in (\"%s\")",
		database,
		metric, strings.Join(keys, "\",\""))
	query := client.NewQuery(influxql, c.Database, c.Precision)
	if response, err := c.Client.Query(query); err == nil && response.Error() == nil {
		tagPairs := make(map[string]*dataobj.TagPair)
		for _, result := range response.Results {
			for _, series := range result.Series {
				for _, valuePair := range series.Values {
					tagKey := valuePair[0].(string)
					tagValue := valuePair[1].(string)
					if pair, exist := tagPairs[tagKey]; exist {
						pair.Values = append(pair.Values, tagValue)
					} else {
						pair := &dataobj.TagPair{
							Key:    tagKey,
							Values: []string{tagValue},
						}
						tagPairs[pair.Key] = pair
						tagkv = append(tagkv, pair)
					}
				}
			}
		}
	}
	return tagkv
}

// show series from metric where ...
func (influxdb *InfluxdbDataSource) QueryIndexByClude(recvs []dataobj.CludeRecv) []dataobj.XcludeResp {
	logger.Debugf("query IndexByClude , recv: %+v", recvs)

	c, err := NewInfluxdbClient(influxdb.Section)
	defer c.Client.Close()

	if err != nil {
		logger.Errorf("init influxdb client fail: %v", err)
		return nil
	}
	resp := make([]dataobj.XcludeResp, 0)
	for _, recv := range recvs {
		xcludeResp := dataobj.XcludeResp{
			Endpoints: recv.Endpoints,
			Metric:    recv.Metric,
			Tags:      make([]string, 0),
			Step:      -1, // fixme
			DsType:    "GAUGE",
		}

		if len(recv.Endpoints) == 0 {
			resp = append(resp, xcludeResp)
			continue
		}

		showSeries := ShowSeries{
			Database:  influxdb.Section.Database,
			Metric:    recv.Metric,
			Endpoints: recv.Endpoints,
			Start:     time.Now().AddDate(0, 0, -30).Unix(),
			End:       time.Now().Unix(),
			Include:   recv.Include,
			Exclude:   recv.Exclude,
		}
		showSeries.renderShow()
		showSeries.renderEndpoints()
		showSeries.renderInclude()
		showSeries.renderExclude()

		query := client.NewQuery(showSeries.RawQuery, c.Database, c.Precision)
		if response, err := c.Client.Query(query); err == nil && response.Error() == nil {
			for _, result := range response.Results {
				for _, series := range result.Series {
					for _, valuePair := range series.Values {

						// proc.port.listen,endpoint=localhost,port=22,service=sshd
						tagKey := valuePair[0].(string)

						// process
						items := strings.Split(tagKey, ",")
						newItems := make([]string, 0)
						for _, item := range items {
							if item != recv.Metric && !strings.Contains(item, "endpoint") {
								newItems = append(newItems, item)
							}
						}

						if len(newItems) > 0 {
							if tags, err := dataobj.SplitTagsString(strings.Join(newItems, ",")); err == nil {
								xcludeResp.Tags = append(xcludeResp.Tags, dataobj.SortedTags(tags))
							}
						}
					}
				}
			}
		}
		resp = append(resp, xcludeResp)
	}

	return resp
}

// show series from metric where ...
func (influxdb *InfluxdbDataSource) QueryIndexByFullTags(recvs []dataobj.IndexByFullTagsRecv) []dataobj.
	IndexByFullTagsResp {
	logger.Debugf("query IndexByFullTags , recv: %+v", recvs)

	c, err := NewInfluxdbClient(influxdb.Section)
	defer c.Client.Close()

	if err != nil {
		logger.Errorf("init influxdb client fail: %v", err)
		return nil
	}

	resp := make([]dataobj.IndexByFullTagsResp, 0)
	for _, recv := range recvs {
		fullTagResp := dataobj.IndexByFullTagsResp{
			Endpoints: recv.Endpoints,
			Metric:    recv.Metric,
			Tags:      make([]string, 0),
			Step:      -1, // FIXME
			DsType:    "GAUGE",
		}

		// 兼容夜莺逻辑，不选择endpoint则返回空
		if len(recv.Endpoints) == 0 {
			resp = append(resp, fullTagResp)
			continue
		}

		// build influxql
		influxdbShow := ShowSeries{
			Database:  influxdb.Section.Database,
			Metric:    recv.Metric,
			Endpoints: recv.Endpoints,
			Start:     time.Now().AddDate(0, 0, -30).Unix(),
			End:       time.Now().Unix(),
		}
		influxdbShow.renderShow()
		influxdbShow.renderEndpoints()
		influxdbShow.renderTimeRange()

		// do query
		query := client.NewQuery(influxdbShow.RawQuery, c.Database, c.Precision)
		if response, err := c.Client.Query(query); err == nil && response.Error() == nil {
			for _, result := range response.Results {
				for _, series := range result.Series {
					for _, valuePair := range series.Values {

						// proc.port.listen,endpoint=localhost,port=22,service=sshd
						tagKey := valuePair[0].(string)

						// process
						items := strings.Split(tagKey, ",")
						newItems := make([]string, 0)
						for _, item := range items {
							if item != recv.Metric && !strings.Contains(item, "endpoint") {
								newItems = append(newItems, item)
							}
						}

						if len(newItems) > 0 {
							if tags, err := dataobj.SplitTagsString(strings.Join(newItems, ",")); err == nil {
								fullTagResp.Tags = append(fullTagResp.Tags, dataobj.SortedTags(tags))
							}
						}
					}
				}
			}
		}
		resp = append(resp, fullTagResp)
	}

	return resp
}

func convertValues(series models.Row) []*dataobj.RRDData {

	// convert values
	values := make([]*dataobj.RRDData, 0, len(series.Values))
	for _, valuePair := range series.Values {
		timestampNumber, _ := valuePair[0].(json.Number)
		timestamp, _ := timestampNumber.Int64()

		valueNumber, _ := valuePair[1].(json.Number)
		valueFloat, _ := valueNumber.Float64()
		values = append(values, dataobj.NewRRDData(timestamp, valueFloat))
	}
	return values
}
