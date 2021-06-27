package models

import (
	"strings"

	"github.com/toolkits/pkg/logger"
)

type MetricDescription struct {
	Id          int64  `json:"id"`
	Metric      string `json:"metric"`
	Description string `json:"description"`
}

func (md *MetricDescription) TableName() string {
	return "metric_description"
}

func MetricDescriptionUpdate(mds []MetricDescription) error {
	for i := 0; i < len(mds); i++ {
		mds[i].Metric = strings.TrimSpace(mds[i].Metric)
		md, err := MetricDescriptionGet("metric = ?", mds[i].Metric)
		if err != nil {
			return err
		}

		if md == nil {
			// insert
			err = DBInsertOne(mds[i])
			if err != nil {
				return err
			}
		} else {
			// update
			md.Description = mds[i].Description
			err = md.Update("description")
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (md *MetricDescription) Update(cols ...string) error {
	_, err := DB.Where("id=?", md.Id).Cols(cols...).Update(md)
	if err != nil {
		logger.Errorf("mysql.error: update metric_description(metric=%s) fail: %v", md.Metric, err)
		return internalServerError
	}

	return nil
}

func MetricDescriptionGet(where string, args ...interface{}) (*MetricDescription, error) {
	var obj MetricDescription
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query metric_description(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func MetricDescriptionTotal(query string) (num int64, err error) {
	if query != "" {
		q := "%" + query + "%"
		num, err = DB.Where("metric like ? or description like ?", q, q).Count(new(MetricDescription))
	} else {
		num, err = DB.Count(new(MetricDescription))
	}

	if err != nil {
		logger.Errorf("mysql.error: count metric_description fail: %v", err)
		return 0, internalServerError
	}

	return num, nil
}

func MetricDescriptionGets(query string, limit, offset int) ([]MetricDescription, error) {
	session := DB.Limit(limit, offset).OrderBy("metric")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("metric like ? or description like ?", q, q)
	}

	var objs []MetricDescription
	err := session.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query metric_description fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []MetricDescription{}, nil
	}

	return objs, nil
}

func MetricDescriptionGetAll() ([]MetricDescription, error) {
	var objs []MetricDescription
	err := DB.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query metric_description fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []MetricDescription{}, nil
	}

	return objs, nil
}

// MetricDescriptionMapper 即时看图页面，应该会用到这个方法，填充metric对应的description
func MetricDescriptionMapper(metrics []string) (map[string]string, error) {
	var objs []MetricDescription
	err := DB.In("metric", metrics).Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query metric_description fail: %v", err)
		return nil, internalServerError
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
	_, err := DB.In("id", ids).Delete(new(MetricDescription))
	if err != nil {
		logger.Errorf("mysql.error: delete metric_description fail: %v", err)
		return internalServerError
	}
	return nil
}
