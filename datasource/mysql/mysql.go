package mysql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dskit/mysql"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/macros"

	"github.com/mitchellh/mapstructure"
	"github.com/toolkits/pkg/logger"
)

const (
	MySQLType = "mysql"
)

func init() {
	datasource.RegisterDatasource(MySQLType, new(MySQL))
}

type MySQL struct {
	mysql.MySQL `json:",inline" mapstructure:",squash"`
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

func (m *MySQL) InitClient() error {
	if len(m.Shards) == 0 {
		return fmt.Errorf("not found mysql addr, please check datasource config")
	}
	if _, err := m.NewConn(context.TODO(), ""); err != nil {
		return err
	}
	return nil
}

func (m *MySQL) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(MySQL)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (m *MySQL) Validate(ctx context.Context) error {
	if len(m.Shards) == 0 || len(strings.TrimSpace(m.Shards[0].Addr)) == 0 {
		return fmt.Errorf("mysql addr is invalid, please check datasource setting")
	}

	if len(strings.TrimSpace(m.Shards[0].User)) == 0 {
		return fmt.Errorf("mysql user is invalid, please check datasource setting")
	}

	return nil
}

// Equal compares whether two objects are the same, used for caching
func (m *MySQL) Equal(p datasource.Datasource) bool {
	newest, ok := p.(*MySQL)
	if !ok {
		logger.Errorf("unexpected plugin type, expected is mysql")
		return false
	}

	if len(m.Shards) == 0 || len(newest.Shards) == 0 {
		return false
	}

	oldShard := m.Shards[0]
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

	return true
}

func (m *MySQL) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (m *MySQL) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (m *MySQL) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (m *MySQL) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	mysqlQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, mysqlQueryParam); err != nil {
		return nil, err
	}

	if strings.Contains(mysqlQueryParam.SQL, "$__") {
		var err error
		mysqlQueryParam.SQL, err = macros.Macro(mysqlQueryParam.SQL, mysqlQueryParam.From, mysqlQueryParam.To)
		if err != nil {
			return nil, err
		}
	}

	if mysqlQueryParam.Keys.ValueKey == "" {
		return nil, fmt.Errorf("valueKey is required")
	}

	timeout := m.Shards[0].Timeout
	if timeout == 0 {
		timeout = 60
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	items, err := m.QueryTimeseries(timeoutCtx, &sqlbase.QueryParam{
		Sql: mysqlQueryParam.SQL,
		Keys: types.Keys{
			ValueKey: mysqlQueryParam.Keys.ValueKey,
			LabelKey: mysqlQueryParam.Keys.LabelKey,
			TimeKey:  mysqlQueryParam.Keys.TimeKey,
		},
	})

	if err != nil {
		logger.Warningf("query:%+v get data err:%v", mysqlQueryParam, err)
		return []models.DataResp{}, err
	}
	data := make([]models.DataResp, 0)
	for i := range items {
		data = append(data, models.DataResp{
			Ref:    mysqlQueryParam.Ref,
			Metric: items[i].Metric,
			Values: items[i].Values,
		})
	}

	return data, nil
}

func (m *MySQL) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	mysqlQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, mysqlQueryParam); err != nil {
		return nil, 0, err
	}

	if strings.Contains(mysqlQueryParam.SQL, "$__") {
		var err error
		mysqlQueryParam.SQL, err = macros.Macro(mysqlQueryParam.SQL, mysqlQueryParam.From, mysqlQueryParam.To)
		if err != nil {
			return nil, 0, err
		}
	}

	timeout := m.Shards[0].Timeout
	if timeout == 0 {
		timeout = 60
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	items, err := m.Query(timeoutCtx, &sqlbase.QueryParam{
		Sql: mysqlQueryParam.SQL,
	})

	if err != nil {
		logger.Warningf("query:%+v get data err:%v", mysqlQueryParam, err)
		return []interface{}{}, 0, err
	}
	logs := make([]interface{}, 0)
	for i := range items {
		logs = append(logs, items[i])
	}

	return logs, 0, nil
}

func (m *MySQL) DescribeTable(ctx context.Context, query interface{}) ([]*types.ColumnProperty, error) {
	mysqlQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, mysqlQueryParam); err != nil {
		return nil, err
	}
	return m.DescTable(ctx, mysqlQueryParam.Database, mysqlQueryParam.Table)
}
