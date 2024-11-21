package datasource

import (
	"context"
	"github.com/ccfos/nightingale/v6/models"
)

type base struct {
}

func (b *base) Init(settings map[string]interface{}) (Datasource, error) {
	panic("implement me")
	return nil, nil
}

func (b *base) InitClient() error {
	panic("implement me")
	return nil
}

func (b *base) Validate(ctx context.Context) error {
	panic("implement me")
	return nil
}

func (b *base) Equal(p Datasource) bool {
	panic("implement me")
	return false
}

func (b *base) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	panic("implement me")
	return nil, nil
}

func (b *base) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	panic("implement me")
	return nil, nil
}

func (b *base) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	panic("implement me")
	return nil, nil
}

func (b *base) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	panic("implement me")
	return nil, 0, nil
}

func (b *base) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	panic("implement me")
	return nil, nil
}
