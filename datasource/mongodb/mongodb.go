package mongodb

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	mongodsk "github.com/ccfos/nightingale/v6/dskit/mongodb"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/mitchellh/mapstructure"
	"github.com/toolkits/pkg/logger"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	MongoDBType = "mongodb"
)

func init() {
	datasource.RegisterDatasource(MongoDBType, new(MongoDB))
}

type MongoDB struct {
	mongodsk.MongoDB `json:",inline" mapstructure:",squash"`
}

type BaseQuery struct {
	Database   string                   `json:"database" mapstructure:"database"`
	Collection string                   `json:"collection" mapstructure:"collection"`
	Pipeline   []map[string]interface{} `json:"pipeline" mapstructure:"pipeline"`
	Filter     map[string]interface{}   `json:"filter" mapstructure:"filter"`
	Projection map[string]interface{}   `json:"projection" mapstructure:"projection"`
	Sort       map[string]interface{}   `json:"sort" mapstructure:"sort"`
	Limit      int64                    `json:"limit" mapstructure:"limit"`
	Skip       int64                    `json:"skip" mapstructure:"skip"`
	TimeField  string                   `json:"time_field" mapstructure:"time_field"`
	From       int64                    `json:"from" mapstructure:"from"`
	To         int64                    `json:"to" mapstructure:"to"`
}

type QueryParam struct {
	BaseQuery `mapstructure:",squash"`
	Ref       string          `json:"ref" mapstructure:"ref"`
	Keys      datasource.Keys `json:"keys" mapstructure:"keys"`
}

type LogQueryParam struct {
	BaseQuery `mapstructure:",squash"`
	Ref       string `json:"ref" mapstructure:"ref"`
}

func (m *MongoDB) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(MongoDB)
	err := mapstructure.WeakDecode(settings, newest)
	return newest, err
}

func (m *MongoDB) InitClient() error {
	if _, err := m.NewClient(context.TODO()); err != nil {
		return err
	}
	return nil
}

func (m *MongoDB) Validate(ctx context.Context) error {
	shard, err := m.firstShard()
	if err != nil {
		return err
	}

	if shard.URI == "" && len(shard.Hosts) == 0 {
		return fmt.Errorf("mongodb uri is invalid, please check datasource setting")
	}

	return nil
}

func (m *MongoDB) Equal(p datasource.Datasource) bool {
	other, ok := p.(*MongoDB)
	if !ok {
		logger.Errorf("unexpected plugin type, expected mongodb")
		return false
	}

	if len(m.Shards) == 0 || len(other.Shards) == 0 {
		return false
	}

	a := m.Shards[0]
	b := other.Shards[0]

	if a.URI != b.URI {
		return false
	}

	if strings.Join(a.Hosts, ",") != strings.Join(b.Hosts, ",") {
		return false
	}

	if a.User != b.User {
		return false
	}

	if a.Password != b.Password {
		return false
	}

	if a.AuthSource != b.AuthSource {
		return false
	}

	if a.ReplicaSet != b.ReplicaSet {
		return false
	}

	if a.Database != b.Database {
		return false
	}

	if a.Timeout != b.Timeout {
		return false
	}

	if a.MaxPoolSize != b.MaxPoolSize {
		return false
	}

	if a.TLSEnable != b.TLSEnable {
		return false
	}

	if a.TLSSkipVerify != b.TLSSkipVerify {
		return false
	}

	if !reflect.DeepEqual(sortedParams(a.Params), sortedParams(b.Params)) {
		return false
	}

	return true
}

func sortedParams(m map[string]string) map[string]string {
	if len(m) == 0 {
		return map[string]string{}
	}

	out := make(map[string]string, len(m))
	for k, v := range m {
		out[strings.ToLower(k)] = v
	}
	return out
}

func (m *MongoDB) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (m *MongoDB) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (m *MongoDB) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (m *MongoDB) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	param := new(QueryParam)
	if err := mapstructure.WeakDecode(query, param); err != nil {
		return nil, err
	}

	if param.Collection == "" {
		return nil, errors.New("collection is required")
	}

	if param.Keys.ValueKey == "" {
		return nil, errors.New("valueKey is required")
	}

	database := param.Database
	if database == "" {
		shard, _ := m.firstShard()
		database = shard.Database
	}

	if database == "" {
		return nil, errors.New("database is required")
	}

	pipeline, err := buildAggregatePipeline(&param.BaseQuery)
	if err != nil {
		return nil, err
	}

	items, err := m.Aggregate(ctx, database, param.Collection, pipeline)
	if err != nil {
		logger.Warningf("query:%+v get data err:%v", param, err)
		return []models.DataResp{}, err
	}

	metrics := sqlbase.FormatMetricValues(types.Keys{
		ValueKey:   param.Keys.ValueKey,
		LabelKey:   param.Keys.LabelKey,
		TimeKey:    param.Keys.TimeKey,
		TimeFormat: param.Keys.TimeFormat,
	}, items)

	resp := make([]models.DataResp, 0, len(metrics))
	for _, metric := range metrics {
		resp = append(resp, models.DataResp{
			Ref:    param.Ref,
			Metric: metric.Metric,
			Values: metric.Values,
		})
	}

	return resp, nil
}

func (m *MongoDB) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	param := new(LogQueryParam)
	if err := mapstructure.WeakDecode(query, param); err != nil {
		return nil, 0, err
	}

	if param.Collection == "" {
		return nil, 0, errors.New("collection is required")
	}

	database := param.Database
	if database == "" {
		shard, _ := m.firstShard()
		database = shard.Database
	}

	if database == "" {
		return nil, 0, errors.New("database is required")
	}

	var (
		data []map[string]interface{}
		err  error
	)

	if len(param.Pipeline) > 0 || len(param.Filter) > 0 || len(param.Sort) > 0 || len(param.Projection) > 0 || param.Limit > 0 || param.Skip > 0 || param.TimeField != "" {
		pipeline, buildErr := buildAggregatePipeline(&param.BaseQuery)
		if buildErr != nil {
			return nil, 0, buildErr
		}
		data, err = m.Aggregate(ctx, database, param.Collection, pipeline)
	} else {
		filter, opts := buildFindRequest(&param.BaseQuery)
		data, err = m.Find(ctx, database, param.Collection, filter, opts)
	}

	if err != nil {
		logger.Warningf("query:%+v get log err:%v", param, err)
		return []interface{}{}, 0, err
	}

	logs := make([]interface{}, len(data))
	for i := range data {
		logs[i] = data[i]
	}

	return logs, int64(len(logs)), nil
}

func buildFindRequest(param *BaseQuery) (interface{}, *options.FindOptions) {
	filter := cloneMap(param.Filter)
	filter = mergeTimeFilter(filter, param)
	normalized := mongodsk.NormalizeValue(filter)
	filterMap, ok := normalized.(map[string]interface{})
	if !ok {
		filterMap = map[string]interface{}{}
	}

	opts := options.Find()
	if len(param.Sort) > 0 {
		opts.SetSort(mongodsk.NormalizeValue(param.Sort))
	}
	if len(param.Projection) > 0 {
		opts.SetProjection(mongodsk.NormalizeValue(param.Projection))
	}
	if param.Limit > 0 {
		opts.SetLimit(param.Limit)
	}
	if param.Skip > 0 {
		opts.SetSkip(param.Skip)
	}

	return filterMap, opts
}

func buildAggregatePipeline(param *BaseQuery) (mongo.Pipeline, error) {
	if len(param.Pipeline) > 0 {
		return mongodsk.ConvertPipeline(param.Pipeline)
	}

	match := mergeTimeFilter(cloneMap(param.Filter), param)
	stageMatch := map[string]interface{}{}
	if len(match) > 0 {
		stageMatch = match
	}

	stages := []map[string]interface{}{
		{"$match": stageMatch},
	}

	if len(param.Sort) > 0 {
		stages = append(stages, map[string]interface{}{"$sort": mongodsk.NormalizeValue(param.Sort)})
	}

	if param.Skip > 0 {
		stages = append(stages, map[string]interface{}{"$skip": param.Skip})
	}

	if param.Limit > 0 {
		stages = append(stages, map[string]interface{}{"$limit": param.Limit})
	}

	if len(param.Projection) > 0 {
		stages = append(stages, map[string]interface{}{"$project": mongodsk.NormalizeValue(param.Projection)})
	}

	return mongodsk.ConvertPipeline(stages)
}

func mergeTimeFilter(filter map[string]interface{}, param *BaseQuery) map[string]interface{} {
	if filter == nil {
		filter = make(map[string]interface{})
	}

	if param.TimeField == "" {
		return filter
	}

	rangeCond := map[string]interface{}{}

	if param.From > 0 {
		if ts, ok := toTime(param.From); ok {
			rangeCond["$gte"] = ts
		} else {
			rangeCond["$gte"] = param.From
		}
	}

	if param.To > 0 {
		if ts, ok := toTime(param.To); ok {
			rangeCond["$lte"] = ts
		} else {
			rangeCond["$lte"] = param.To
		}
	}

	if len(rangeCond) == 0 {
		return filter
	}

	if existing, ok := filter[param.TimeField]; ok {
		if existingMap, ok := existing.(map[string]interface{}); ok {
			for k, v := range rangeCond {
				existingMap[k] = v
			}
			filter[param.TimeField] = existingMap
			return filter
		}
	}

	filter[param.TimeField] = rangeCond
	return filter
}

func toTime(ts int64) (time.Time, bool) {
	if ts <= 0 {
		return time.Time{}, false
	}

	if ts > 1e12 {
		return time.UnixMilli(ts), true
	}

	return time.Unix(ts, 0), true
}

func (m *MongoDB) firstShard() (*mongodsk.Shard, error) {
	if len(m.Shards) == 0 {
		return nil, errors.New("empty mongodb shards")
	}

	return &m.Shards[0], nil
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	if len(m) == 0 {
		return make(map[string]interface{})
	}

	cp := make(map[string]interface{}, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
