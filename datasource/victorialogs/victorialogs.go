package victorialogs

import (
	"context"
	"fmt"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dskit/victorialogs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/mitchellh/mapstructure"
)

type VictoriaLogs struct {
	client *victorialogs.VictoriaLogsClient
}

func init() {
	datasource.RegisterDatasource("victorialogs", new(VictoriaLogs))
}

func (v *VictoriaLogs) InitClient() error {
	return v.client.InitCli()
}

func (v *VictoriaLogs) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	vl := &VictoriaLogs{}
	vl.client = &victorialogs.VictoriaLogsClient{}
	if err := mapstructure.Decode(settings, vl.client); err != nil {
		return nil, fmt.Errorf("failed to decode victoria logs datasource settings: %v", err)
	}
	return vl, nil
}

func (v *VictoriaLogs) Validate(ctx context.Context) error {
	return v.client.InitCli()
}

func (v *VictoriaLogs) Equal(p datasource.Datasource) bool {
	other, ok := p.(*VictoriaLogs)
	if !ok {
		return false
	}
	return v.client.Equal(other.client)
}

func (v *VictoriaLogs) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	// map参数转换为结构体
	mq := &victorialogs.QueryParam{}
	if err := mapstructure.Decode(query, mq); err != nil {
		return nil, fmt.Errorf("failed to decode log query: %v", err)
	}
	if start != 0 {
		mq.Start = start
	}
	if end != 0 {
		mq.End = end
	}
	return mq, nil
}

func (v *VictoriaLogs) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (v *VictoriaLogs) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (v *VictoriaLogs) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	return nil, nil
}

func (v *VictoriaLogs) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	queryMap, ok := query.(map[string]interface{})
	if !ok {
		return nil, 0, fmt.Errorf("invalid query parameter")
	}
	qp := &victorialogs.QueryParam{}
	if err := mapstructure.Decode(queryMap, qp); err != nil {
		return nil, 0, fmt.Errorf("failed to decode log query: %v", err)
	}
	data, err := v.client.QueryLogs(ctx, qp)
	if err != nil {
		return nil, 0, err
	}
	total, err := v.client.HitsLogs(ctx, qp)
	if err != nil {
		return nil, 0, err
	}
	results := make([]interface{}, len(data))
	for i, d := range data {
		results[i] = d
	}
	return results, total, nil
}
