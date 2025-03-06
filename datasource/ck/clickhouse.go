package ck

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/datasource"
	ck "github.com/ccfos/nightingale/v6/dskit/clickhouse"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/macros"

	"github.com/mitchellh/mapstructure"
	"github.com/toolkits/pkg/logger"
)

const (
	CKType = "ck"

	TimeFieldFormatEpochMilli  = "epoch_millis"
	TimeFieldFormatEpochSecond = "epoch_second"

	DefaultLimit = 500
)

var (
	ckPrivBanned = []string{
		"INSERT",
		"CREATE",
		"DROP",
		"DELETE",
		"UPDATE",
		"ALL",
	}

	ckBannedOp = map[string]struct{}{
		"CREATE":   {},
		"INSERT":   {},
		"ALTER":    {},
		"REVOKE":   {},
		"DROP":     {},
		"RENAME":   {},
		"ATTACH":   {},
		"DETACH":   {},
		"OPTIMIZE": {},
		"TRUNCATE": {},
		"SET":      {},
	}
)

func init() {
	datasource.RegisterDatasource(CKType, new(Clickhouse))
}

type CKShard struct {
	Addr        string `json:"ck.addr" mapstructure:"ck.addr"`
	User        string `json:"ck.user" mapstructure:"ck.user"`
	Password    string `json:"ck.password" mapstructure:"ck.password"`
	Database    string `json:"ck.db" mapstructure:"ck.db"`
	IsEncrypted bool   `json:"ck.is_encrypt" mapstructure:"ck.is_encrypt"`
}

type QueryParam struct {
	Limit      int             `json:"limit" mapstructure:"limit"`
	Sql        string          `json:"sql" mapstructure:"sql"`
	Ref        string          `json:"ref" mapstructure:"ref"`
	From       int64           `json:"from" mapstructure:"from"`
	To         int64           `json:"to" mapstructure:"to"`
	TimeField  string          `json:"time_field" mapstructure:"time_field"`
	TimeFormat string          `json:"time_format" mapstructure:"time_format"`
	Keys       datasource.Keys `json:"keys" mapstructure:"keys"`
	Database   string          `json:"database" mapstructure:"database"`
	Table      string          `json:"table" mapstructure:"table"`
}

type Clickhouse struct {
	ck.Clickhouse `json:",inline" mapstructure:",squash"`
}

func (c *Clickhouse) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(Clickhouse)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (c *Clickhouse) InitClient() error {
	return c.InitCli()
}

func (c *Clickhouse) Validate(ctx context.Context) error {
	if len(c.Nodes) == 0 {
		return fmt.Errorf("ck shard is invalid, please check datasource setting")
	}

	addr := c.Nodes[0]
	if len(strings.Trim(c.User, " ")) == 0 {
		return fmt.Errorf("ck shard user is invalid, please check datasource setting")
	}

	if len(strings.Trim(addr, " ")) == 0 {
		return fmt.Errorf("ck shard addr is invalid, please check datasource setting")
	}

	// if len(strings.Trim(shard.Password, " ")) == 0 {
	// 	return fmt.Errorf("ck shard password is empty, please check datasource setting or set password for user")
	// }

	return nil
}

// Equal compares whether two objects are the same, used for caching
func (c *Clickhouse) Equal(p datasource.Datasource) bool {

	plg, ok := p.(*Clickhouse)
	if !ok {
		logger.Errorf("unexpected plugin type, expected is ck")
		return false
	}

	// only compare first shard
	if len(c.Nodes) == 0 {
		logger.Errorf("ck shard is empty")
		return false
	}
	addr := c.Nodes[0]

	if len(plg.Nodes) == 0 {
		logger.Errorf("new ck plugin obj shard is empty")
		return false
	}
	newAddr := plg.Nodes[0]

	if c.User != plg.User {
		return false
	}

	if addr != newAddr {
		return false
	}

	if c.Password != plg.Password {
		return false
	}

	return true
}

func (c *Clickhouse) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (c *Clickhouse) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (c *Clickhouse) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (c *Clickhouse) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {

	ckQueryParam := new(ck.QueryParam)
	if err := mapstructure.Decode(query, ckQueryParam); err != nil {
		return nil, err
	}

	if strings.Contains(ckQueryParam.Sql, "$__") {
		var err error
		ckQueryParam.Sql, err = macros.Macro(ckQueryParam.Sql, ckQueryParam.From, ckQueryParam.To)
		if err != nil {
			return nil, err
		}
	}

	if ckQueryParam.Keys.ValueKey == "" {
		return nil, fmt.Errorf("valueKey is required")
	}

	rows, err := c.QueryTimeseries(ctx, ckQueryParam)
	if err != nil {
		logger.Warningf("query:%+v get data err:%v", ckQueryParam, err)
		return nil, err
	}

	if err != nil {
		logger.Warningf("query:%+v get data err:%v", ckQueryParam, err)
		return []models.DataResp{}, err
	}
	data := make([]models.DataResp, 0)
	for i := range rows {
		data = append(data, models.DataResp{
			Ref:    ckQueryParam.Ref,
			Metric: rows[i].Metric,
			Values: rows[i].Values,
		})
	}

	return data, nil
}

func (c *Clickhouse) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	ckQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, ckQueryParam); err != nil {
		return nil, 0, err
	}

	if strings.Contains(ckQueryParam.Sql, "$__") {
		var err error
		ckQueryParam.Sql, err = macros.Macro(ckQueryParam.Sql, ckQueryParam.From, ckQueryParam.To)
		if err != nil {
			return nil, 0, err
		}
	}

	rows, err := c.Query(ctx, ckQueryParam)
	if err != nil {
		logger.Warningf("query:%+v get data err:%v", ckQueryParam, err)
		return nil, 0, err
	}

	limit := getLimit(len(rows), ckQueryParam.Limit)

	logs := make([]interface{}, 0)
	for i := 0; i < limit; i++ {
		logs = append(logs, rows[i])
	}

	return logs, int64(limit), nil
}

func getLimit(rowLen, pLimit int) int {
	limit := DefaultLimit
	if pLimit > 0 {
		limit = pLimit
	}
	if rowLen > limit {
		return limit
	}

	return rowLen
}
