package memsto

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/container/set"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

const SYSTEM = "system"

type BuiltinComponentCacheType struct {
	statTotal              int64
	statLastUpdated        int64
	ctx                    *ctx.Context
	stats                  *Stats
	builtinIntegrationsDir string // path to the directory containing builtin components, e.g., "/path/to/builtin/components"

	sync.RWMutex
	bc map[uint64]*models.BuiltinComponent // key: id
	bp map[int64]*models.BuiltinPayload    // key: id
}

func NewBuiltinComponentCache(ctx *ctx.Context, stats *Stats, builtinIntegrationsDir string) *BuiltinComponentCacheType {
	bc := &BuiltinComponentCacheType{
		statTotal:              -1,
		statLastUpdated:        -1,
		ctx:                    ctx,
		stats:                  stats,
		builtinIntegrationsDir: builtinIntegrationsDir,
		bc:                     make(map[uint64]*models.BuiltinComponent),
		bp:                     make(map[int64]*models.BuiltinPayload),
	}

	bc.SyncBuiltinComponents()
	return bc
}
func (b *BuiltinComponentCacheType) StatChanged(total, lastUpdated int64) bool {
	if b.statTotal == total && b.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (b *BuiltinComponentCacheType) SyncBuiltinComponents() {
	b.initBuiltinComponentFiles()

	err := b.syncBuiltinComponents()
	if err != nil {
		logger.Errorf("failed to sync builtin components: %v", err)
	}

	go b.loopSyncBuiltinComponents()
}

func (b *BuiltinComponentCacheType) initBuiltinComponentFiles() error {
	fp := b.builtinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}

	// var fileList []string
	dirList, err := file.DirsUnder(fp)
	if err != nil {
		logger.Warning("read builtin component dir fail ", err)
		return err
	}

	for _, dir := range dirList {
		// components icon
		componentDir := fp + "/" + dir
		component := models.BuiltinComponent{
			Ident: dir,
		}

		// get logo name
		// /api/n9e/integrations/icon/AliYun/aliyun.png
		files, err := file.FilesUnder(componentDir + "/icon")
		if err == nil && len(files) > 0 {
			component.Logo = "/api/n9e/integrations/icon/" + component.Ident + "/" + files[0]
		} else if err != nil {
			logger.Warningf("read builtin component icon dir fail %s %v", component.Ident, err)
		}

		// get description
		files, err = file.FilesUnder(componentDir + "/markdown")
		if err == nil && len(files) > 0 {
			var readmeFile string
			for _, file := range files {
				if strings.HasSuffix(strings.ToLower(file), "md") {
					readmeFile = componentDir + "/markdown/" + file
					break
				}
			}
			if readmeFile != "" {
				component.Readme, _ = file.ReadString(readmeFile)
			}
		} else if err != nil {
			logger.Warningf("read builtin component markdown dir fail %s %v", component.Ident, err)
		}

		// alerts
		files, err = file.FilesUnder(componentDir + "/alerts")
		if err == nil && len(files) > 0 {
			for _, f := range files {
				fp := componentDir + "/alerts/" + f
				bs, err := file.ReadBytes(fp)
				if err != nil {
					logger.Warning("read builtin component alerts file fail ", f, err)
					continue
				}

				alerts := []models.AlertRule{}
				err = json.Unmarshal(bs, &alerts)
				if err != nil {
					logger.Warning("parse builtin component alerts file fail ", f, err)
					continue
				}

				for _, alert := range alerts {
					if alert.UUID == 0 {
						alert.UUID = time.Now().UnixNano()
					}

					content, err := json.Marshal(alert)
					if err != nil {
						logger.Warning("marshal builtin alert fail ", alert, err)
						continue
					}

					cate := strings.Replace(f, ".json", "", -1)
					builtinAlert := models.BuiltinPayload{
						ComponentID: component.ID,
						Type:        "alert",
						Cate:        cate,
						Name:        alert.Name,
						Tags:        alert.AppendTags,
						Content:     string(content),
						UUID:        alert.UUID,
					}

					b.addBuiltinPayload(&builtinAlert)
				}
			}
		}

		// dashboards
		files, err = file.FilesUnder(componentDir + "/dashboards")
		if err == nil && len(files) > 0 {
			for _, f := range files {
				fp := componentDir + "/dashboards/" + f
				bs, err := file.ReadBytes(fp)
				if err != nil {
					logger.Warning("read builtin component dashboards file fail ", f, err)
					continue
				}

				dashboard := BuiltinBoard{}
				err = json.Unmarshal(bs, &dashboard)
				if err != nil {
					logger.Warning("parse builtin component dashboards file fail ", f, err)
					continue
				}

				if dashboard.UUID == 0 {
					time.Sleep(time.Microsecond)
					dashboard.UUID = time.Now().UnixMicro()
					bs, err = json.MarshalIndent(dashboard, "", "    ")
					if err != nil {
						logger.Warning("marshal builtin dashboard fail ", dashboard, err)
						continue
					}

					_, err = file.WriteBytes(fp, bs)
					if err != nil {
						logger.Warning("write builtin dashboard file fail ", f, err)
					}
				}

				content, err := json.Marshal(dashboard)
				if err != nil {
					logger.Warning("marshal builtin dashboard fail ", dashboard, err)
					continue
				}

				builtinDashboard := models.BuiltinPayload{
					ComponentID: component.ID,
					Type:        "dashboard",
					Cate:        "",
					Name:        dashboard.Name,
					Tags:        dashboard.Tags,
					Content:     string(content),
					UUID:        dashboard.UUID,
				}

				b.addBuiltinPayload(&builtinDashboard)
			}
		} else if err != nil {
			logger.Warningf("read builtin component dash dir fail %s %v", component.Ident, err)
		}
	}

	return nil
}

func (b *BuiltinComponentCacheType) loopSyncBuiltinComponents() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := b.syncBuiltinComponents(); err != nil {
			logger.Warning("failed to sync datasources:", err)
		}
	}
}

func (b *BuiltinComponentCacheType) syncBuiltinComponents() error {
	start := time.Now()

	stat, err := models.BuiltinComponentStatistics(b.ctx)
	if err != nil {
		dumper.PutSyncRecord("builtin_components", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec BuiltinComponentStatistics")
	}

	if !b.StatChanged(stat.Total, stat.LastUpdated) {
		b.stats.GaugeCronDuration.WithLabelValues("sync_builtin_components").Set(0)
		b.stats.GaugeSyncNumber.WithLabelValues("sync_builtin_components").Set(0)
		dumper.PutSyncRecord("builtin_components", start.Unix(), -1, -1, "not changed")
		return nil
	}

	bc, err := models.BuiltinComponentGetAllMap(b.ctx)
	if err != nil {
		dumper.PutSyncRecord("builtin_components", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call BuiltinComponentGetMap")
	}

	b.Set(bc, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	b.stats.GaugeCronDuration.WithLabelValues("sync_builtin_components").Set(float64(ms))
	b.stats.GaugeSyncNumber.WithLabelValues("sync_builtin_components").Set(float64(len(bc)))

	logger.Infof("timer: sync builtin components done, cost: %dms, number: %d", ms, len(bc))
	dumper.PutSyncRecord("builtin_components", start.Unix(), ms, len(bc), "success")

	return nil
}

func (b *BuiltinComponentCacheType) Set(bc map[uint64]*models.BuiltinComponent, total, lastUpdated int64) {
	b.Lock()
	b.bc = bc
	b.Unlock()

	// only one goroutine used, so no need lock
	b.statTotal = total
	b.statLastUpdated = lastUpdated
}

func (b *BuiltinComponentCacheType) GetByBuiltinComponentId(id uint64) *models.BuiltinComponent {
	b.RLock()
	defer b.RLock()
	return b.bc[id]
}

func (b *BuiltinComponentCacheType) GetNamesByBuiltinComponentIds(ids []uint64) []string {
	b.RLock()
	defer b.RLock()
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = b.bc[id].Ident
	}
	return names
}

func (b *BuiltinComponentCacheType) GetBuiltinPayload(typ, cate, query string, componentId uint64) ([]*models.BuiltinPayload, error) {
	var result []*models.BuiltinPayload

	// TODO: Use table to speed up query
	for _, payload := range b.bp {
		if (typ != "" && payload.Type != typ) ||
			(componentId != 0 && payload.ComponentID != componentId) ||
			(cate != "" && payload.Cate != cate) ||
			(query != "" && !strings.Contains(payload.Name, query) && !strings.Contains(payload.Tags, query)) {
			continue
		}

		result = append(result, payload)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no results found")
	}

	return result, nil
}

func (b *BuiltinComponentCacheType) GetBuiltinPayloadById(id int64) (*models.BuiltinPayload, error) {
	b.RLock()
	defer b.RUnlock()
	payload, ok := b.bp[id]
	if ok {
		return payload, nil
	}

	return nil, fmt.Errorf("no results found")
}

func (b *BuiltinComponentCacheType) GetBuiltinPayloadCates(typ string, componentId uint64) ([]string, error) {
	var result set.StringSet

	// TODO: Use table to speed up query
	for _, payload := range b.bp {
		if (typ != "" && payload.Type != typ) ||
			(componentId != 0 && payload.ComponentID != componentId) {
			continue
		}

		result = *result.Add(payload.Cate)
	}

	resultStrings := result.ToSlice()
	if len(resultStrings) == 0 {
		return nil, fmt.Errorf("no results found")
	}

	return resultStrings, nil
}

func (b *BuiltinComponentCacheType) GetNameByBuiltinComponentId(id uint64) string {
	b.RLock()
	defer b.RUnlock()
	if bc, exists := b.bc[id]; exists {
		return bc.Ident
	}
	return ""
}

func (b *BuiltinComponentCacheType) addBuiltinComponent(bc *models.BuiltinComponent) error {
	b.Lock()
	defer b.Unlock()

	if _, exists := b.bc[bc.ID]; exists {
		return errors.New("builtin component already exists")
	}

	b.bc[bc.ID] = bc
	b.statTotal++
	b.statLastUpdated = time.Now().Unix()

	return nil
}

func (b *BuiltinComponentCacheType) addBuiltinPayload(bp *models.BuiltinPayload) error {
	b.Lock()
	defer b.Unlock()

	if _, exists := b.bp[bp.ID]; exists {
		return errors.New("builtin payload already exists")
	}

	b.bp[bp.ID] = bp
	return nil
}

type BuiltinBoard struct {
	Id         int64       `json:"id" gorm:"primaryKey"`
	GroupId    int64       `json:"group_id"`
	Name       string      `json:"name"`
	Ident      string      `json:"ident"`
	Tags       string      `json:"tags"`
	CreateAt   int64       `json:"create_at"`
	CreateBy   string      `json:"create_by"`
	UpdateAt   int64       `json:"update_at"`
	UpdateBy   string      `json:"update_by"`
	Configs    interface{} `json:"configs" gorm:"-"`
	Public     int         `json:"public"`      // 0: false, 1: true
	PublicCate int         `json:"public_cate"` // 0: anonymous, 1: login, 2: busi
	Bgids      []int64     `json:"bgids" gorm:"-"`
	BuiltIn    int         `json:"built_in"` // 0: false, 1: true
	Hide       int         `json:"hide"`     // 0: false, 1: true
	UUID       int64       `json:"uuid"`
}
