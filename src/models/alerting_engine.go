package models

import "time"

type AlertingEngines struct {
	Id       int64  `json:"id" gorm:"primaryKey"`
	Instance string `json:"instance"`
	Cluster  string `json:"cluster"` // reader cluster
	Clock    int64  `json:"clock"`
}

func (e *AlertingEngines) TableName() string {
	return "alerting_engines"
}

// UpdateCluster 页面上用户会给各个n9e-server分配要关联的目标集群是什么
func (e *AlertingEngines) UpdateCluster(c string) error {
	e.Cluster = c
	return DB().Model(e).Select("cluster").Updates(e).Error
}

// AlertingEngineGetCluster 根据实例名获取对应的集群名字
func AlertingEngineGetCluster(instance string) (string, error) {
	var objs []AlertingEngines
	err := DB().Where("instance=?", instance).Find(&objs).Error
	if err != nil {
		return "", err
	}

	if len(objs) == 0 {
		return "", nil
	}

	return objs[0].Cluster, nil
}

// AlertingEngineGets 拉取列表数据，用户要在页面上看到所有 n9e-server 实例列表，然后为其分配 cluster
func AlertingEngineGets(where string, args ...interface{}) ([]*AlertingEngines, error) {
	var objs []*AlertingEngines
	var err error
	session := DB().Order("instance")
	if where == "" {
		err = session.Find(&objs).Error
	} else {
		err = session.Where(where, args...).Find(&objs).Error
	}
	return objs, err
}

func AlertingEngineGet(where string, args ...interface{}) (*AlertingEngines, error) {
	lst, err := AlertingEngineGets(where, args...)
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func AlertingEngineGetsInstances(where string, args ...interface{}) ([]string, error) {
	var arr []string
	var err error
	session := DB().Model(new(AlertingEngines)).Order("instance")
	if where == "" {
		err = session.Pluck("instance", &arr).Error
	} else {
		err = session.Where(where, args...).Pluck("instance", &arr).Error
	}
	return arr, err
}

func AlertingEngineHeartbeat(instance, cluster string) error {
	var total int64
	err := DB().Model(new(AlertingEngines)).Where("instance=?", instance).Count(&total).Error
	if err != nil {
		return err
	}

	if total == 0 {
		// insert
		err = DB().Create(&AlertingEngines{
			Instance: instance,
			Cluster:  cluster,
			Clock:    time.Now().Unix(),
		}).Error
	} else {
		// updates
		fields := map[string]interface{}{"clock": time.Now().Unix(), "cluster": cluster}
		err = DB().Model(new(AlertingEngines)).Where("instance=?", instance).Updates(fields).Error
	}

	return err
}
