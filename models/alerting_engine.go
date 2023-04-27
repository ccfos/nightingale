package models

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type AlertingEngines struct {
	Id            int64  `json:"id" gorm:"primaryKey"`
	Instance      string `json:"instance"`
	EngineCluster string `json:"cluster" gorm:"engine_cluster"`
	DatasourceId  int64  `json:"datasource_id"`
	Clock         int64  `json:"clock"`
}

func (e *AlertingEngines) TableName() string {
	return "alerting_engines"
}

// UpdateCluster 页面上用户会给各个n9e-server分配要关联的目标集群是什么
func (e *AlertingEngines) UpdateDatasourceId(ctx *ctx.Context, id int64) error {
	count, err := Count(DB(ctx).Model(&AlertingEngines{}).Where("id<>? and instance=? and datasource_id=?", e.Id, e.Instance, id))
	if err != nil {
		return err
	}

	if count > 0 {
		return fmt.Errorf("instance %s and datasource_id %d already exists", e.Instance, id)
	}

	e.DatasourceId = id
	return DB(ctx).Model(e).Select("datasource_id").Updates(e).Error
}

func AlertingEngineAdd(ctx *ctx.Context, instance string, datasourceId int64) error {
	count, err := Count(DB(ctx).Model(&AlertingEngines{}).Where("instance=? and datasource_id=?", instance, datasourceId))
	if err != nil {
		return err
	}

	if count > 0 {
		return fmt.Errorf("instance %s and datasource_id %d already exists", instance, datasourceId)
	}

	err = DB(ctx).Create(&AlertingEngines{
		Instance:     instance,
		DatasourceId: datasourceId,
		Clock:        time.Now().Unix(),
	}).Error

	return err
}

func AlertingEngineDel(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB(ctx).Where("id in ?", ids).Delete(new(AlertingEngines)).Error
}

func AlertingEngineGetDatasourceIds(ctx *ctx.Context, instance string) ([]int64, error) {
	var objs []AlertingEngines
	err := DB(ctx).Where("instance=?", instance).Find(&objs).Error
	if err != nil {
		return []int64{}, err
	}

	if len(objs) == 0 {
		return []int64{}, nil
	}
	var ids []int64
	for i := 0; i < len(objs); i++ {
		ids = append(ids, objs[i].DatasourceId)
	}

	return ids, nil
}

// AlertingEngineGets 拉取列表数据，用户要在页面上看到所有 n9e-server 实例列表，然后为其分配 cluster
func AlertingEngineGets(ctx *ctx.Context, where string, args ...interface{}) ([]*AlertingEngines, error) {
	var objs []*AlertingEngines
	var err error
	session := DB(ctx).Order("instance")
	if where == "" {
		err = session.Find(&objs).Error
	} else {
		err = session.Where(where, args...).Find(&objs).Error
	}
	return objs, err
}

func AlertingEngineGet(ctx *ctx.Context, where string, args ...interface{}) (*AlertingEngines, error) {
	lst, err := AlertingEngineGets(ctx, where, args...)
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func AlertingEngineGetsClusters(ctx *ctx.Context, where string, args ...interface{}) ([]string, error) {
	var arr []string
	var err error
	session := DB(ctx).Model(new(AlertingEngines)).Where("engine_cluster != ''").Order("engine_cluster").Distinct("engine_cluster")
	if where == "" {
		err = session.Pluck("engine_cluster", &arr).Error
	} else {
		err = session.Where(where, args...).Pluck("engine_cluster", &arr).Error
	}
	return arr, err
}

func AlertingEngineGetsInstances(ctx *ctx.Context, where string, args ...interface{}) ([]string, error) {
	var arr []string
	var err error
	session := DB(ctx).Model(new(AlertingEngines)).Order("instance")
	if where == "" {
		err = session.Pluck("instance", &arr).Error
	} else {
		err = session.Where(where, args...).Pluck("instance", &arr).Error
	}
	return arr, err
}

func AlertingEngineHeartbeatWithCluster(ctx *ctx.Context, instance, cluster string, datasourceId int64) error {
	var total int64
	err := DB(ctx).Model(new(AlertingEngines)).Where("instance=? and engine_cluster = ? and datasource_id=?", instance, cluster, datasourceId).Count(&total).Error
	if err != nil {
		return err
	}

	if total == 0 {
		// insert
		err = DB(ctx).Create(&AlertingEngines{
			Instance:      instance,
			DatasourceId:  datasourceId,
			EngineCluster: cluster,
			Clock:         time.Now().Unix(),
		}).Error
	} else {
		// updates
		fields := map[string]interface{}{"clock": time.Now().Unix()}
		err = DB(ctx).Model(new(AlertingEngines)).Where("instance=? and engine_cluster = ? and datasource_id=?", instance, cluster, datasourceId).Updates(fields).Error
	}

	return err
}
