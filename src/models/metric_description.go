package models

import (
	"strings"
	"time"
)

type MetricDescription struct {
	Id          int64  `json:"id"`
	Metric      string `json:"metric"`
	Description string `json:"description"`
	UpdateAt    int64  `json:"update_at"`
}

func (md *MetricDescription) TableName() string {
	return "metric_description"
}

func MetricDescriptionUpdate(mds []MetricDescription) error {
	now := time.Now().Unix()

	for i := 0; i < len(mds); i++ {
		mds[i].Metric = strings.TrimSpace(mds[i].Metric)
		md, err := MetricDescriptionGet("metric = ?", mds[i].Metric)
		if err != nil {
			return err
		}

		if md == nil {
			// insert
			mds[i].UpdateAt = now
			err = Insert(&mds[i])
			if err != nil {
				return err
			}
		} else {
			// update
			err = md.Update(mds[i].Description, now)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (md *MetricDescription) Update(desn string, now int64) error {
	md.Description = desn
	md.UpdateAt = now
	return DB().Model(md).Select("Description", "UpdateAt").Updates(md).Error
}

func MetricDescriptionGet(where string, args ...interface{}) (*MetricDescription, error) {
	var lst []*MetricDescription
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func MetricDescriptionTotal(query string) (int64, error) {
	session := DB().Model(&MetricDescription{})

	if query != "" {
		q := "%" + query + "%"
		session = session.Where("metric like ? or description like ?", q, q)
	}

	return Count(session)
}

func MetricDescriptionGets(query string, limit, offset int) ([]MetricDescription, error) {
	session := DB().Order("metric").Limit(limit).Offset(offset)
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("metric like ? or description like ?", q, q)
	}

	var objs []MetricDescription
	err := session.Find(&objs).Error
	return objs, err
}

func MetricDescGetAll() ([]MetricDescription, error) {
	var objs []MetricDescription
	err := DB().Find(&objs).Error
	return objs, err
}

func MetricDescStatistics() (*Statistics, error) {
	session := DB().Model(&MetricDescription{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func MetricDescriptionMapper(metrics []string) (map[string]string, error) {
	if len(metrics) == 0 {
		return map[string]string{}, nil
	}

	var objs []MetricDescription
	err := DB().Where("metric in ?", metrics).Find(&objs).Error
	if err != nil {
		return nil, err
	}

	count := len(objs)
	if count == 0 {
		return map[string]string{}, nil
	}

	mapper := make(map[string]string, count)
	for i := 0; i < count; i++ {
		mapper[objs[i].Metric] = objs[i].Description
	}

	return mapper, nil
}

func MetricDescriptionDel(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	return DB().Where("id in ?", ids).Delete(new(MetricDescription)).Error
}
