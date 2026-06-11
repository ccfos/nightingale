package oracle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dskit/oracle"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/macros"

	"github.com/mitchellh/mapstructure"
	"github.com/toolkits/pkg/logger"

	"github.com/ccfos/nightingale/v6/pkg/logx"
)

const (
	OracleType = "oracle"
)

func init() {
	datasource.RegisterDatasource(OracleType, new(Oracle))
}

type Oracle struct {
	oracle.Oracle `json:",inline" mapstructure:",squash"`
}

type QueryParam struct {
	Ref      string          `json:"ref" mapstructure:"ref"`
	Database string          `json:"database" mapstructure:"database"`
	Table    string          `json:"table" mapstructure:"table"`
	SQL      string          `json:"sql" mapstructure:"sql"`
	Keys     datasource.Keys `json:"keys" mapstructure:"keys"`
	From     int64           `json:"from" mapstructure:"from"`
	To       int64           `json:"to" mapstructure:"to"`
}

func (o *Oracle) InitClient() error {
	if len(o.Shards) == 0 {
		return fmt.Errorf("not found oracle addr, please check datasource config")
	}
	if _, err := o.NewConn(context.TODO(), ""); err != nil {
		return err
	}
	return nil
}

func (o *Oracle) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(Oracle)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (o *Oracle) Validate(ctx context.Context) error {
	if len(o.Shards) == 0 || len(strings.TrimSpace(o.Shards[0].Addr)) == 0 {
		return fmt.Errorf("oracle addr is invalid, please check datasource setting")
	}

	if len(strings.TrimSpace(o.Shards[0].User)) == 0 {
		return fmt.Errorf("oracle user is invalid, please check datasource setting")
	}

	return nil
}

func (o *Oracle) Equal(p datasource.Datasource) bool {
	newest, ok := p.(*Oracle)
	if !ok {
		logger.Errorf("unexpected plugin type, expected is oracle")
		return false
	}

	if len(o.Shards) == 0 || len(newest.Shards) == 0 {
		return false
	}

	oldShard := o.Shards[0]
	newShard := newest.Shards[0]

	if oldShard.Addr != newShard.Addr {
		return false
	}

	if oldShard.User != newShard.User {
		return false
	}

	if oldShard.Password != newShard.Password {
		return false
	}

	if oldShard.MaxQueryRows != newShard.MaxQueryRows {
		return false
	}

	if oldShard.Timeout != newShard.Timeout {
		return false
	}

	if oldShard.MaxIdleConns != newShard.MaxIdleConns {
		return false
	}

	if oldShard.MaxOpenConns != newShard.MaxOpenConns {
		return false
	}

	if oldShard.ConnMaxLifetime != newShard.ConnMaxLifetime {
		return false
	}

	return true
}

func (o *Oracle) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (o *Oracle) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (o *Oracle) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (o *Oracle) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	oracleQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, oracleQueryParam); err != nil {
		return nil, err
	}

	if strings.Contains(oracleQueryParam.SQL, "$__") {
		var err error
		oracleQueryParam.SQL, err = macros.Macro(oracleQueryParam.SQL, oracleQueryParam.From, oracleQueryParam.To)
		if err != nil {
			return nil, err
		}
	}

	if oracleQueryParam.Keys.ValueKey == "" {
		return nil, fmt.Errorf("valueKey is required")
	}

	timeout := o.Shards[0].Timeout
	if timeout == 0 {
		timeout = 60
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	items, err := o.QueryTimeseries(timeoutCtx, &sqlbase.QueryParam{
		Sql: oracleQueryParam.SQL,
		Keys: types.Keys{
			ValueKey: oracleQueryParam.Keys.ValueKey,
			LabelKey: oracleQueryParam.Keys.LabelKey,
			TimeKey:  oracleQueryParam.Keys.TimeKey,
		},
	})

	if err != nil {
		logx.Warningf(ctx, "query:%+v get data err:%v", oracleQueryParam, err)
		return []models.DataResp{}, err
	}
	data := make([]models.DataResp, 0)
	for i := range items {
		data = append(data, models.DataResp{
			Ref:    oracleQueryParam.Ref,
			Metric: items[i].Metric,
			Values: items[i].Values,
		})
	}

	return data, nil
}

func (o *Oracle) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	oracleQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, oracleQueryParam); err != nil {
		return nil, 0, err
	}

	if strings.Contains(oracleQueryParam.SQL, "$__") {
		var err error
		oracleQueryParam.SQL, err = macros.Macro(oracleQueryParam.SQL, oracleQueryParam.From, oracleQueryParam.To)
		if err != nil {
			return nil, 0, err
		}
	}

	timeout := o.Shards[0].Timeout
	if timeout == 0 {
		timeout = 60
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	items, err := o.Query(timeoutCtx, &sqlbase.QueryParam{
		Sql: oracleQueryParam.SQL,
	})

	if err != nil {
		logx.Warningf(ctx, "query:%+v get data err:%v", oracleQueryParam, err)
		return []interface{}{}, 0, err
	}
	logs := make([]interface{}, 0)
	for i := range items {
		logs = append(logs, items[i])
	}

	return logs, 0, nil
}

func (o *Oracle) DescribeTable(ctx context.Context, query interface{}) ([]*types.ColumnProperty, error) {
	oracleQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, oracleQueryParam); err != nil {
		return nil, err
	}
	return o.DescTable(ctx, oracleQueryParam.Database, oracleQueryParam.Table)
}
