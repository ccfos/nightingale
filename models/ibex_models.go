package models

import (
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/ibex/server/config"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"gorm.io/gorm"
)

func IbexCount(tx *gorm.DB) (int64, error) {
	var cnt int64
	err := tx.Count(&cnt).Error
	return cnt, err
}

func tht(id int64) string {
	return fmt.Sprintf("task_host_%d", id%100)
}

func TableRecordGets[T any](ctx *ctx.Context, table, where string, args ...interface{}) (lst T, err error) {
	if config.C.IsCenter {
		if where == "" || len(args) == 0 {
			err = DB(ctx).Table(table).Find(&lst).Error
		} else {
			err = DB(ctx).Table(table).Where(where, args...).Find(&lst).Error
		}
		return
	}

	return poster.PostByUrlsWithResp[T](ctx, "/ibex/v1/table/record/list", map[string]interface{}{
		"table": table,
		"where": where,
		"args":  args,
	})
}

func TableRecordCount(ctx *ctx.Context, table, where string, args ...interface{}) (int64, error) {
	if config.C.IsCenter {
		if where == "" || len(args) == 0 {
			return IbexCount(DB(ctx).Table(table))
		}
		return IbexCount(DB(ctx).Table(table).Where(where, args...))
	}

	return poster.PostByUrlsWithResp[int64](ctx, "/ibex/v1/table/record/count", map[string]interface{}{
		"table": table,
		"where": where,
		"args":  args,
	})
}

var IBEX_HOST_DOING = "ibex-host-doing"

func CacheRecordGets[T any](ctx *ctx.Context) ([]T, error) {
	lst := make([]T, 0)
	values, _ := ctx.Redis.HVals(ctx.Ctx, IBEX_HOST_DOING).Result()
	for _, val := range values {
		t := new(T)
		if err := json.Unmarshal([]byte(val), t); err != nil {
			return nil, err
		}
		lst = append(lst, *t)
	}
	return lst, nil
}
