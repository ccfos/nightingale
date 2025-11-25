package victorialogs

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dskit/victorialogs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/mitchellh/mapstructure"
)

type VictoriaLogs struct {
	victorialogs.VictoriaLogsClient `json:",inline" mapstructure:",squash"`
}

func init() {
	datasource.RegisterDatasource("victorialogs", new(VictoriaLogs))
}

func (v *VictoriaLogs) InitClient() error {
	return v.InitCli()
}

func (v *VictoriaLogs) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(VictoriaLogs)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (v *VictoriaLogs) Validate(ctx context.Context) error {
	return v.VictoriaLogsClient.Validate()
}

func (v *VictoriaLogs) Equal(other datasource.Datasource) bool {
	otherDs, ok := other.(*VictoriaLogs)
	if !ok {
		return false
	}
	return v.Url == otherDs.Url &&
		v.Username == otherDs.Username &&
		v.Password == otherDs.Password &&
		v.SkipTLSVerify == otherDs.SkipTLSVerify &&
		v.DialTimeout == otherDs.DialTimeout &&
		v.MaxIdleConnsPerHost == otherDs.MaxIdleConnsPerHost &&
		reflect.DeepEqual(v.Headers, otherDs.Headers)
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

func buildDataRespFromMetricAndValueSlices(qp *victorialogs.QueryParam, metric map[string]interface{}, valueSlices [][]interface{}) models.DataResp {
	dr := models.DataResp{
		Values: make([][]float64, 0, len(valueSlices)),
		Ref:    qp.Ref,
	}
	_ = mapstructure.Decode(metric, &dr.Metric)

	for _, val := range valueSlices {
		if val == nil {
			continue
		}
		if v2, err := ToValue(val); err == nil {
			dr.Values = append(dr.Values, v2)
		}
	}
	return dr
}

func (v *VictoriaLogs) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	queryMap, ok := query.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid query parameter")
	}
	qp := &victorialogs.QueryParam{}
	if err := mapstructure.Decode(queryMap, qp); err != nil {
		return nil, fmt.Errorf("failed to decode data query: %v", err)
	}

	if qp.IsInstantQuery() {
		data, err := v.StatsQuery(ctx, qp)
		if err != nil {
			return nil, err
		}
		result := make([]models.DataResp, 0, len(data.Data.Result))
		for _, item := range data.Data.Result {
			// item.Value is a single [ts, value], wrap into [][]interface{} for reuse
			valueSlices := [][]interface{}{}
			if item.Value != nil {
				valueSlices = append(valueSlices, item.Value)
			}
			dr := buildDataRespFromMetricAndValueSlices(qp, item.Metric, valueSlices)
			result = append(result, dr)
		}
		return result, nil
	} else {
		data, err := v.StatsQueryRange(ctx, qp)
		if err != nil {
			return nil, err
		}
		result := make([]models.DataResp, 0, len(data.Data.Result))
		for _, item := range data.Data.Result {
			// Values [][]interface{}
			dr := buildDataRespFromMetricAndValueSlices(qp, item.Metric, item.Values)
			result = append(result, dr)
		}
		return result, nil
	}
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
	data, err := v.QueryLogs(ctx, qp)
	if err != nil {
		return nil, 0, err
	}
	total, err := v.HitsLogs(ctx, qp)
	if err != nil {
		return nil, 0, err
	}
	results := make([]interface{}, len(data))
	for i, d := range data {
		results[i] = d
	}
	return results, total, nil
}

// ToValue 负责把[ts: float64, value: string] 转换为 []float64
func ToValue(arr []interface{}) ([]float64, error) {
	if len(arr) != 2 {
		return nil, fmt.Errorf("invalid value array length")
	}
	ts, ok := arr[0].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid timestamp type")
	}
	var valueFloat float64
	switch v := arr[1].(type) {
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid value string: %v", err)
		}
		valueFloat = f
	case float64:
		valueFloat = v
	default:
		return nil, fmt.Errorf("invalid value type")
	}
	return []float64{ts, valueFloat}, nil
}
