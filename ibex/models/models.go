package models

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/ibex/pkg/poster"
	"github.com/ccfos/nightingale/v6/ibex/server/config"
	"github.com/ccfos/nightingale/v6/ibex/storage"

	"gorm.io/gorm"
)

func DB() *gorm.DB {
	return storage.DB
}

func Count(tx *gorm.DB) (int64, error) {
	var cnt int64
	err := tx.Count(&cnt).Error
	return cnt, err
}

func Exists(tx *gorm.DB) (bool, error) {
	num, err := Count(tx)
	return num > 0, err
}

func Insert(objPtr interface{}) error {
	return DB().Create(objPtr).Error
}

func tht(id int64) string {
	return fmt.Sprintf("task_host_%d", id%100)
}

func TableRecordGets[T any](table, where string, args ...interface{}) (lst T, err error) {
	if config.C.IsCenter {
		if where == "" || len(args) == 0 {
			err = DB().Table(table).Find(&lst).Error
		} else {
			err = DB().Table(table).Where(where, args...).Find(&lst).Error
		}
		return
	}

	return poster.PostByUrlsWithResp[T](config.C.CenterApi, "/ibex/v1/table/record/list", map[string]interface{}{
		"table": table,
		"where": where,
		"args":  args,
	})
}

func TableRecordCount(table, where string, args ...interface{}) (int64, error) {
	if config.C.IsCenter {
		if where == "" || len(args) == 0 {
			return Count(DB().Table(table))
		}
		return Count(DB().Table(table).Where(where, args...))
	}

	return poster.PostByUrlsWithResp[int64](config.C.CenterApi, "/ibex/v1/table/record/count", map[string]interface{}{
		"table": table,
		"where": where,
		"args":  args,
	})
}

var IBEX_HOST_DOING = "ibex-host-doing"

func CacheRecordGets[T any](ctx context.Context) ([]T, error) {
	lst := make([]T, 0)
	values, _ := storage.Cache.HVals(ctx, IBEX_HOST_DOING).Result()
	for _, val := range values {
		t := new(T)
		if err := json.Unmarshal([]byte(val), t); err != nil {
			return nil, err
		}
		lst = append(lst, *t)
	}
	return lst, nil
}
