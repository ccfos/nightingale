package models

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/ibex/server/config"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaskHost struct {
	II     int64  `gorm:"column:ii;primaryKey;autoIncrement" json:"-"`
	Id     int64  `gorm:"column:id;uniqueIndex:idx_id_host;not null" json:"id"`
	Host   string `gorm:"column:host;uniqueIndex:idx_id_host;size:128;not null" json:"host"`
	Status string `gorm:"column:status;size:32;not null" json:"status"`
	Stdout string `gorm:"column:stdout;type:text" json:"stdout"`
	Stderr string `gorm:"column:stderr;type:text" json:"stderr"`
}

func (taskHost *TaskHost) Upsert(ctx *ctx.Context) error {
	return DB(ctx).Table(tht(taskHost.Id)).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}, {Name: "host"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "stdout", "stderr"}),
	}).Create(taskHost).Error
}

func (taskHost *TaskHost) Create(ctx *ctx.Context) error {
	if config.C.IsCenter {
		return DB(ctx).Table(tht(taskHost.Id)).Create(taskHost).Error
	}
	return poster.PostByUrls(ctx, "/ibex/v1/task/host", taskHost)
}

func TaskHostUpserts(ctx *ctx.Context, lst []TaskHost) (map[string]error, error) {
	if len(lst) == 0 {
		return nil, fmt.Errorf("empty list")
	}

	if !config.C.IsCenter {
		return poster.PostByUrlsWithResp[map[string]error](ctx, "/ibex/v1/task/hosts/upsert", lst)
	}

	errs := make(map[string]error, 0)
	for _, taskHost := range lst {
		if err := taskHost.Upsert(ctx); err != nil {
			errs[fmt.Sprintf("%d:%s", taskHost.Id, taskHost.Host)] = err
		}
	}
	return errs, nil
}

func TaskHostGet(ctx *ctx.Context, id int64, host string) (*TaskHost, error) {
	var ret []*TaskHost
	err := DB(ctx).Table(tht(id)).Where("id=? and host=?", id, host).Find(&ret).Error
	if err != nil {
		return nil, err
	}

	if len(ret) == 0 {
		return nil, nil
	}

	return ret[0], nil
}

func MarkDoneStatus(ctx *ctx.Context, id, clock int64, host, status, stdout, stderr string, edgeAlertTriggered ...bool) error {
	if len(edgeAlertTriggered) > 0 && edgeAlertTriggered[0] {
		return CacheMarkDone(ctx, TaskHost{
			Id:     id,
			Host:   host,
			Status: status,
			Stdout: stdout,
			Stderr: stderr,
		})
	}

	if !config.C.IsCenter {
		return poster.PostByUrls(ctx, "/ibex/v1/mark/done", map[string]interface{}{
			"id":     id,
			"clock":  clock,
			"host":   host,
			"status": status,
			"stdout": stdout,
			"stderr": stderr,
		})
	}

	count, err := TableRecordCount(ctx, TaskHostDoing{}.TableName(), "id=? and host=? and clock=?", id, host, clock)
	if err != nil {
		return err
	}

	if count == 0 {
		// 如果是timeout了，后来任务执行完成之后，结果又上来了，stdout和stderr最好还是存库，让用户看到
		count, err = TableRecordCount(ctx, tht(id), "id=? and host=? and status=?", id, host, "timeout")
		if err != nil {
			return err
		}

		if count == 1 {
			return DB(ctx).Table(tht(id)).Where("id=? and host=?", id, host).Updates(map[string]interface{}{
				"status": status,
				"stdout": stdout,
				"stderr": stderr,
			}).Error
		}
		return nil
	}

	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		err = tx.Table(tht(id)).Where("id=? and host=?", id, host).Updates(map[string]interface{}{
			"status": status,
			"stdout": stdout,
			"stderr": stderr,
		}).Error
		if err != nil {
			return err
		}

		if err = tx.Where("id=? and host=?", id, host).Delete(&TaskHostDoing{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func RealTimeUpdateOutput(ctx *ctx.Context, id int64, host, stdout, stderr string) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Table(tht(id)).Where("id=? and host=?", id, host).Updates(map[string]interface{}{
			"stdout": stdout,
			"stderr": stderr,
		}).Error
		if err != nil {
			return err
		}

		return nil
	})
}

func CacheMarkDone(ctx *ctx.Context, taskHost TaskHost) error {
	if err := ctx.Redis.HDel(ctx.Ctx, IBEX_HOST_DOING, hostDoingCacheKey(taskHost.Id, taskHost.Host)).Err(); err != nil {
		return err
	}
	TaskHostCachePush(taskHost)

	return nil
}

func WaitingHostList(ctx *ctx.Context, id int64, limit ...int) ([]TaskHost, error) {
	var hosts []TaskHost
	session := DB(ctx).Table(tht(id)).Where("id = ? and status = 'waiting'", id).Order("ii")
	if len(limit) > 0 {
		session = session.Limit(limit[0])
	}
	err := session.Find(&hosts).Error
	return hosts, err
}

func WaitingHostCount(ctx *ctx.Context, id int64) (int64, error) {
	return TableRecordCount(ctx, tht(id), "id=? and status='waiting'", id)
}

func UnexpectedHostCount(ctx *ctx.Context, id int64) (int64, error) {
	return TableRecordCount(ctx, tht(id), "id=? and status in ('failed', 'timeout', 'killfailed')", id)
}

func IngStatusHostCount(ctx *ctx.Context, id int64) (int64, error) {
	return TableRecordCount(ctx, tht(id), "id=? and status in ('waiting', 'running', 'killing')", id)
}

func RunWaitingHosts(ctx *ctx.Context, taskHosts []TaskHost) error {
	count := len(taskHosts)
	if count == 0 {
		return nil
	}

	now := time.Now().Unix()

	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		for i := 0; i < count; i++ {
			if err := tx.Table(tht(taskHosts[i].Id)).Where("id=? and host=?", taskHosts[i].Id, taskHosts[i].Host).Update("status", "running").Error; err != nil {
				return err
			}
			err := tx.Create(&TaskHostDoing{Id: taskHosts[i].Id, Host: taskHosts[i].Host, Clock: now, Action: "start"}).Error
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func TaskHostStatus(ctx *ctx.Context, id int64) ([]TaskHost, error) {
	var ret []TaskHost
	err := DB(ctx).Table(tht(id)).Select("id", "host", "status").Where("id=?", id).Order("ii").Find(&ret).Error
	return ret, err
}

func TaskHostGets(ctx *ctx.Context, id int64) ([]TaskHost, error) {
	var ret []TaskHost
	err := DB(ctx).Table(tht(id)).Where("id=?", id).Order("ii").Find(&ret).Error
	return ret, err
}

var (
	taskHostCache = make([]TaskHost, 0, 128)
	taskHostLock  sync.RWMutex
)

func TaskHostCachePush(taskHost TaskHost) {
	taskHostLock.Lock()
	defer taskHostLock.Unlock()

	taskHostCache = append(taskHostCache, taskHost)
}

func TaskHostCachePopAll() []TaskHost {
	taskHostLock.Lock()
	defer taskHostLock.Unlock()

	all := taskHostCache
	taskHostCache = make([]TaskHost, 0, 128)

	return all
}

func ReportCacheResult(ctx *ctx.Context) error {
	result := TaskHostCachePopAll()
	reports := make([]TaskHost, 0)
	for _, th := range result {
		// id大于redis初始id，说明是edge与center失联时，本地告警规则触发的自愈脚本生成的id
		// 为了防止不同边缘机房生成的脚本任务id相同，不上报结果至数据库
		if th.Id >= storage.IDINITIAL {
			logger.Infof("task[%d] host[%s] done, result:[%v]", th.Id, th.Host, th)
		} else {
			reports = append(reports, th)
		}
	}

	if len(reports) == 0 {
		return nil
	}

	errs, err := TaskHostUpserts(ctx, reports)
	if err != nil {
		return err
	}
	for key, err := range errs {
		logger.Warningf("report task_host_cache[%s] result error: %v", key, err)
	}
	return nil
}
