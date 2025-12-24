package doris

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dskit/doris"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/macros"

	"github.com/mitchellh/mapstructure"
	"github.com/toolkits/pkg/logger"
)

const (
	DorisType = "doris"
)

func init() {
	datasource.RegisterDatasource(DorisType, new(Doris))
}

type Doris struct {
	doris.Doris `json:",inline" mapstructure:",squash"`
}

type QueryParam struct {
	Ref        string          `json:"ref" mapstructure:"ref"`
	Database   string          `json:"database" mapstructure:"database"`
	Table      string          `json:"table" mapstructure:"table"`
	SQL        string          `json:"sql" mapstructure:"sql"`
	Keys       datasource.Keys `json:"keys" mapstructure:"keys"`
	Limit      int             `json:"limit" mapstructure:"limit"`
	From       int64           `json:"from" mapstructure:"from"`
	To         int64           `json:"to" mapstructure:"to"`
	TimeField  string          `json:"time_field" mapstructure:"time_field"`
	TimeFormat string          `json:"time_format" mapstructure:"time_format"`
	Interval   int64           `json:"interval" mapstructure:"interval"` // 查询时间间隔（秒）
	Offset     int             `json:"offset" mapstructure:"offset"`     // 延迟计算，不在使用通用配置delay
}

func (d *Doris) InitClient() error {
	if len(d.Addr) == 0 {
		return fmt.Errorf("not found doris addr, please check datasource config")
	}
	if _, err := d.NewConn(context.TODO(), ""); err != nil {
		return err
	}
	return nil
}

func (d *Doris) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(Doris)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (d *Doris) Validate(ctx context.Context) error {
	if len(d.Addr) == 0 || len(strings.TrimSpace(d.Addr)) == 0 {
		return fmt.Errorf("doris addr is invalid, please check datasource setting")
	}

	if len(strings.TrimSpace(d.User)) == 0 {
		return fmt.Errorf("doris user is invalid, please check datasource setting")
	}

	return nil
}

// Equal compares whether two objects are the same, used for caching
func (d *Doris) Equal(p datasource.Datasource) bool {
	newest, ok := p.(*Doris)
	if !ok {
		logger.Errorf("unexpected plugin type, expected is doris")
		return false
	}

	// only compare first shard
	if d.Addr != newest.Addr {
		return false
	}

	if d.User != newest.User {
		return false
	}

	if d.Password != newest.Password {
		return false
	}

	if d.EnableWrite != newest.EnableWrite {
		return false
	}

	if d.FeAddr != newest.FeAddr {
		return false
	}

	if d.MaxQueryRows != newest.MaxQueryRows {
		return false
	}

	if d.Timeout != newest.Timeout {
		return false
	}

	if d.MaxIdleConns != newest.MaxIdleConns {
		return false
	}

	if d.MaxOpenConns != newest.MaxOpenConns {
		return false
	}

	if d.ConnMaxLifetime != newest.ConnMaxLifetime {
		return false
	}

	if d.ClusterName != newest.ClusterName {
		return false
	}

	return true
}

func (d *Doris) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (d *Doris) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (d *Doris) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (d *Doris) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	dorisQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, dorisQueryParam); err != nil {
		return nil, err
	}

	if dorisQueryParam.Keys.ValueKey == "" {
		return nil, fmt.Errorf("valueKey is required")
	}

	// 设置默认 interval
	if dorisQueryParam.Interval == 0 {
		dorisQueryParam.Interval = 60
	}

	// 计算时间范围
	now := time.Now().Unix()
	var start, end int64
	if dorisQueryParam.To != 0 && dorisQueryParam.From != 0 {
		end = dorisQueryParam.To
		start = dorisQueryParam.From
	} else {
		end = now
		start = end - dorisQueryParam.Interval
	}

	if dorisQueryParam.Offset != 0 {
		end -= int64(dorisQueryParam.Offset)
		start -= int64(dorisQueryParam.Offset)
	}

	dorisQueryParam.From = start
	dorisQueryParam.To = end

	if strings.Contains(dorisQueryParam.SQL, "$__") {
		var err error
		dorisQueryParam.SQL, err = macros.Macro(dorisQueryParam.SQL, dorisQueryParam.From, dorisQueryParam.To)
		if err != nil {
			return nil, err
		}
	}

	items, err := d.QueryTimeseries(context.TODO(), &doris.QueryParam{
		Database: dorisQueryParam.Database,
		Sql:      dorisQueryParam.SQL,
		Keys: types.Keys{
			ValueKey: dorisQueryParam.Keys.ValueKey,
			LabelKey: dorisQueryParam.Keys.LabelKey,
			TimeKey:  dorisQueryParam.Keys.TimeKey,
			Offset:   dorisQueryParam.Offset,
		},
	})
	if err != nil {
		logger.Warningf("query:%+v get data err:%v", dorisQueryParam, err)
		return []models.DataResp{}, err
	}
	data := make([]models.DataResp, 0)
	for i := range items {
		data = append(data, models.DataResp{
			Ref:    dorisQueryParam.Ref,
			Metric: items[i].Metric,
			Values: items[i].Values,
		})
	}

	// parse resp to time series data
	logger.Infof("req:%+v keys:%+v \n data:%v", dorisQueryParam, dorisQueryParam.Keys, data)

	return data, nil
}

func (d *Doris) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	dorisQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, dorisQueryParam); err != nil {
		return nil, 0, err
	}

	// 记录规则预览，只传了interval, 没有传From和To //
	now := time.Now().Unix()
	if dorisQueryParam.To == 0 && dorisQueryParam.From == 0 && dorisQueryParam.Interval != 0 {
		dorisQueryParam.To = now
		dorisQueryParam.From = now - dorisQueryParam.Interval
	}

	if dorisQueryParam.Offset != 0 {
		dorisQueryParam.To -= int64(dorisQueryParam.Offset)
		dorisQueryParam.From -= int64(dorisQueryParam.Offset)
	}
	// 记录规则预览，只传了interval, 没有传From和To //

	if strings.Contains(dorisQueryParam.SQL, "$__") {
		var err error
		dorisQueryParam.SQL, err = macros.Macro(dorisQueryParam.SQL, dorisQueryParam.From, dorisQueryParam.To)
		if err != nil {
			return nil, 0, err
		}
	}

	items, err := d.QueryLogs(ctx, &doris.QueryParam{
		Database: dorisQueryParam.Database,
		Sql:      dorisQueryParam.SQL,
	})
	if err != nil {
		logger.Warningf("query:%+v get data err:%v", dorisQueryParam, err)
		return []interface{}{}, 0, err
	}
	logs := make([]interface{}, 0)
	for i := range items {
		logs = append(logs, items[i])
	}

	return logs, int64(len(logs)), nil
}

func (d *Doris) DescribeTable(ctx context.Context, query interface{}) ([]*types.ColumnProperty, error) {
	dorisQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, dorisQueryParam); err != nil {
		return nil, err
	}
	return d.DescTable(ctx, dorisQueryParam.Database, dorisQueryParam.Table)
}
