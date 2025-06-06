package memsto

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
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
	bc          map[uint64]*models.BuiltinComponent // key: id
	bpsInSystem map[int64]*models.BuiltinPayload    // key: id, all builtin payloads
	bpsInUser   map[int64]*models.BuiltinPayload    // key: id, all builtin payloads
	// Created by system, do no need to be synced.
	bpInSystem map[uint64]map[string][]*models.BuiltinPayload // key: id => map[cate][]payload
	// Created by user, need to be synced with the database
	bpInUser map[uint64]map[string][]*models.BuiltinPayload // key: id => map[cate][]payload
}

func NewBuiltinComponentCache(ctx *ctx.Context, stats *Stats, builtinIntegrationsDir string) *BuiltinComponentCacheType {
	bc := &BuiltinComponentCacheType{
		statTotal:              -1,
		statLastUpdated:        -1,
		ctx:                    ctx,
		stats:                  stats,
		builtinIntegrationsDir: builtinIntegrationsDir,
		bc:                     make(map[uint64]*models.BuiltinComponent),
		bpsInSystem:            make(map[int64]*models.BuiltinPayload),
		bpsInUser:              make(map[int64]*models.BuiltinPayload),
		bpInSystem:             make(map[uint64]map[string][]*models.BuiltinPayload),
		bpInUser:               make(map[uint64]map[string][]*models.BuiltinPayload),
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

	err = b.syncBuiltinPayloads()
	if err != nil {
		logger.Errorf("failed to sync builtin payload: %v", err)
	}

	go b.loopSyncBuiltinComponentsAndPayloads()
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

					b.addBuiltinPayload(&builtinAlert, true)
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

				b.addBuiltinPayload(&builtinDashboard, true)
			}
		} else if err != nil {
			logger.Warningf("read builtin component dash dir fail %s %v", component.Ident, err)
		}
	}

	return nil
}

func (b *BuiltinComponentCacheType) loopSyncBuiltinComponentsAndPayloads() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := b.syncBuiltinComponents(); err != nil {
			logger.Warning("failed to sync datasources:", err)
		}
		if err := b.syncBuiltinPayloads(); err != nil {
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

	b.SetBuiltinComponent(bc, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	b.stats.GaugeCronDuration.WithLabelValues("sync_builtin_components").Set(float64(ms))
	b.stats.GaugeSyncNumber.WithLabelValues("sync_builtin_components").Set(float64(len(bc)))

	logger.Infof("timer: sync builtin components done, cost: %dms, number: %d", ms, len(bc))
	dumper.PutSyncRecord("builtin_components", start.Unix(), ms, len(bc), "success")

	return nil
}

func (b *BuiltinComponentCacheType) syncBuiltinPayloads() error {
	start := time.Now()

	stat, err := models.BuiltinPayloadsStatistics(b.ctx)
	if err != nil {
		dumper.PutSyncRecord("builtin_payloads", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec BuiltinPayloadsStatistics")
	}

	if !b.StatChanged(stat.Total, stat.LastUpdated) {
		b.stats.GaugeCronDuration.WithLabelValues("sync_builtin_payloads").Set(0)
		b.stats.GaugeSyncNumber.WithLabelValues("sync_builtin_payloads").Set(0)
		dumper.PutSyncRecord("builtin_payloads", start.Unix(), -1, -1, "not changed")
		return nil
	}

	bc, err := models.BuiltinPayloadsGetAllMap(b.ctx)
	if err != nil {
		dumper.PutSyncRecord("builtin_payloads", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call BuiltinPayloadsGetAllMap")
	}

	b.SetBuiltinPayload(bc, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	b.stats.GaugeCronDuration.WithLabelValues("sync_builtin_payloads").Set(float64(ms))
	b.stats.GaugeSyncNumber.WithLabelValues("sync_builtin_payloads").Set(float64(len(bc)))

	logger.Infof("timer: sync builtin payloads done, cost: %dms, number: %d", ms, len(bc))
	dumper.PutSyncRecord("builtin_payloads", start.Unix(), ms, len(bc), "success")

	return nil
}

func (b *BuiltinComponentCacheType) SetBuiltinComponent(bc map[uint64]*models.BuiltinComponent, total, lastUpdated int64) {
	b.Lock()
	b.bc = bc
	b.Unlock()

	// only one goroutine used, so no need lock
	b.statTotal = total
	b.statLastUpdated = lastUpdated
}

func (b *BuiltinComponentCacheType) BuiltinComponentGets(query string, disabled int, isByUser bool) ([]*models.BuiltinComponent, error) {
	var lst []*models.BuiltinComponent
	for _, component := range b.bc {
		if query != "" && !strings.Contains(component.Ident, query) {
			continue
		}
		if disabled == 0 && component.Disabled != disabled {
			continue
		}
		if disabled == 1 && component.Disabled != disabled {
			continue
		}
		if isByUser && component.CreatedBy == SYSTEM {
			continue
		}
		lst = append(lst, component)
	}

	sort.Slice(lst, func(i, j int) bool {
		return lst[i].Ident < lst[j].Ident
	})

	return lst, nil
}

// SetBuiltinPayload sets the builtin payloads in the cache, only for payloads created by user.
func (b *BuiltinComponentCacheType) SetBuiltinPayload(bp map[int64]*models.BuiltinPayload, total, lastUpdated int64) {
	for _, payload := range bp {
		if payload.CreatedBy == SYSTEM {
			b.addBuiltinPayload(payload, true)
			continue
		} else {
			b.addBuiltinPayload(payload, false)
		}
	}

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
	b.RLock()
	defer b.RUnlock()

	bpInCateInSystem, okInSystem := b.bpInSystem[componentId]
	bpInCateInUser, okInUser := b.bpInUser[componentId]
	if !okInSystem && !okInUser {
		return nil, fmt.Errorf("no builtin payloads found for component id %d", componentId)
	}

	if okInSystem {
		if cate == "" {
			for _, bps := range bpInCateInSystem {
				for _, bp := range bps {
					if query != "" && !strings.Contains(bp.Name, query) && !strings.Contains(bp.Tags, query) {
						continue
					}
					if typ != "" && bp.Type != typ {
						continue
					}
					result = append(result, bp)
				}
			}
		} else {
			bps, exists := bpInCateInSystem[cate]
			if exists {
				for _, bp := range bps {
					if query != "" && !strings.Contains(bp.Name, query) && !strings.Contains(bp.Tags, query) {
						continue
					}
					if typ != "" && bp.Type != typ {
						continue
					}
					result = append(result, bp)
				}
			}
		}
	}

	if okInUser {
		if cate == "" {
			for _, bps := range bpInCateInUser {
				for _, bp := range bps {
					if query != "" && !strings.Contains(bp.Name, query) && !strings.Contains(bp.Tags, query) {
						continue
					}
					if typ != "" && bp.Type != typ {
						continue
					}
					result = append(result, bp)
				}
			}
		} else {
			bps, exists := bpInCateInUser[cate]
			if exists {
				for _, bp := range bps {
					if query != "" && !strings.Contains(bp.Name, query) && !strings.Contains(bp.Tags, query) {
						continue
					}
					if typ != "" && bp.Type != typ {
						continue
					}
					result = append(result, bp)
				}
			}
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no results found")
	}

	return result, nil
}

// GetBuiltinPayloadByUUID returns the builtin payload by uuid
// This function is in low performance, better not to use it in high frequency.
func (b *BuiltinComponentCacheType) GetBuiltinPayloadByUUID(uuid int64) (*models.BuiltinPayload, error) {
	b.RLock()
	defer b.RUnlock()

	for _, payload := range b.bpsInSystem {
		if payload.UUID == uuid {
			return payload, nil
		}
	}

	for _, payload := range b.bpsInUser {
		if payload.UUID == uuid {
			return payload, nil
		}
	}

	return nil, fmt.Errorf("no results found")
}

func (b *BuiltinComponentCacheType) GetBuiltinPayloadById(id int64) (*models.BuiltinPayload, error) {
	b.RLock()
	defer b.RUnlock()

	payloadInSystem, okInSystem := b.bpsInSystem[id]
	if okInSystem {
		return payloadInSystem, nil
	}
	payloadInUser, okInUser := b.bpsInUser[id]
	if okInUser {
		return payloadInUser, nil
	}

	return nil, fmt.Errorf("no results found")
}

func (b *BuiltinComponentCacheType) GetBuiltinPayloadCates(typ string, componentId uint64) ([]string, error) {
	logger.Infof("b.bpInSystem: %d typ: %s componentId: %d", len(b.bpInSystem), typ, componentId)
	var result set.StringSet

	bpInCateInSystem, okInSystem := b.bpInSystem[componentId]
	bpInCateInUser, okInUser := b.bpInUser[componentId]

	if !okInSystem && !okInUser {
		return nil, fmt.Errorf("no builtin payloads found for component id %d", componentId)
	}

	if okInSystem {
		for cate, bps := range bpInCateInSystem {
			if typ != "" && bps[0].Type != typ {
				continue
			}
			if componentId != 0 && bps[0].ComponentID != componentId {
				continue
			}
			result.Add(cate)
		}
	}

	if okInUser {
		for cate, bps := range bpInCateInUser {
			if typ != "" && bps[0].Type != typ {
				continue
			}
			if componentId != 0 && bps[0].ComponentID != componentId {
				continue
			}
			result.Add(cate)
		}
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

// addBuiltinPayload adds a new builtin payload to the cache.
// If the payload is created by the system, it is stored in bpInSystem.
// If it is created by the user, it is stored in bpInUser.
// This function will not check duplicates.
func (b *BuiltinComponentCacheType) addBuiltinPayload(bp *models.BuiltinPayload, isSystem bool) {
	b.Lock()
	defer b.Unlock()

	if isSystem {
		bpInSystem, exists := b.bpInSystem[bp.ComponentID]
		if !exists {
			bpInSystem := make(map[string][]*models.BuiltinPayload)
			bpInSystem[bp.Cate] = append(bpInSystem[bp.Cate], bp)
			b.bpInSystem[bp.ComponentID] = bpInSystem
			return
		}
		bpInCateInSystem, exists := bpInSystem[bp.Cate]
		if !exists {
			bpInCateInSystem = []*models.BuiltinPayload{bp}
			bpInSystem[bp.Cate] = bpInCateInSystem
			b.bpInSystem[bp.ComponentID] = bpInSystem
			return
		}
		bpInSystem[bp.Cate] = append(bpInCateInSystem, bp)
		b.bpInSystem[bp.ComponentID] = bpInSystem

		// Add key value data to bpsInSystem
		b.bpsInSystem[bp.ID] = bp
	} else {
		bpInUser, exists := b.bpInUser[bp.ComponentID]
		if !exists {
			bpInUser := make(map[string][]*models.BuiltinPayload)
			bpInUser[bp.Cate] = append(bpInUser[bp.Cate], bp)
			b.bpInUser[bp.ComponentID] = bpInUser
			return
		}
		bpInCateInUser, exists := bpInUser[bp.Cate]
		if !exists {
			bpInCateInUser = []*models.BuiltinPayload{bp}
			bpInUser[bp.Cate] = bpInCateInUser
			b.bpInUser[bp.ComponentID] = bpInUser
			return
		}
		bpInUser[bp.Cate] = append(bpInCateInUser, bp)
		b.bpInUser[bp.ComponentID] = bpInUser

		// Add key value data to bpsInUser
		b.bpsInUser[bp.ID] = bp
	}
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
