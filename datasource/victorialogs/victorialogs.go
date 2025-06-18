package victorialogs

import (
	"context"
	"reflect"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/mitchellh/mapstructure"

	"github.com/toolkits/pkg/logger"
)

const (
	VictorialogsType = "victorialogs"
)

func init() {
	datasource.RegisterDatasource(VictorialogsType, new(Victorialogs))
}

type Victorialogs struct {
	Addr         string                 `json:"addr"`
	Start        float64                `json:"start"`
	End          float64                `json:"end"`
	CustomParams map[string]interface{} `json:"custom_params"`
}

func (v *Victorialogs) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(Victorialogs)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (v *Victorialogs) InitClient() error {
	return nil
}

func (v *Victorialogs) Validate(ctx context.Context) error {
	return nil
}

func (v *Victorialogs) Equal(p datasource.Datasource) bool {
	newest, ok := p.(*Victorialogs)
	if !ok {
		logger.Errorf("unexpected plugin type, expected is victorialogs")
		return false
	}
	if v.Addr != newest.Addr {
		return false
	}
	if v.Start != newest.Start {
		return false
	}
	if v.End != newest.End {
		return false
	}
	if !reflect.DeepEqual(v.CustomParams, newest.CustomParams) {
		return false
	}
	return true
}

func (v *Victorialogs) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (v *Victorialogs) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (v *Victorialogs) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	return nil, nil
}

func (v *Victorialogs) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	return nil, 0, nil
}

func (v *Victorialogs) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}