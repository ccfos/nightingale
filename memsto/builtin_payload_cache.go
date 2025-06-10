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

type BuiltinPayloadCacheType struct {
	statTotal              int64
	statLastUpdated        int64
	ctx                    *ctx.Context
	stats                  *Stats
	builtinIntegrationsDir string // path to the directory containing builtin components, e.g., "/path/to/builtin/components"

	sync.RWMutex
	// Created from files, do no need to be synced.
	buildPayloadsByFile map[uint64]map[string]map[string][]*models.BuiltinPayload // map[componet_id]map[type]map[cate][]*models.BuiltinPayload
	// Created from db, need to be synced with the database
	buildPayloadsByDB map[uint64]map[string]map[string][]*models.BuiltinPayload // map[componet_id]map[type]map[cate][]*models.BuiltinPayload
}

func NewBuiltinPayloadCache(ctx *ctx.Context, stats *Stats, builtinIntegrationsDir string) *BuiltinPayloadCacheType {
	bc := &BuiltinPayloadCacheType{
		statTotal:              -1,
		statLastUpdated:        -1,
		ctx:                    ctx,
		stats:                  stats,
		builtinIntegrationsDir: builtinIntegrationsDir,
		buildPayloadsByFile:    make(map[uint64]map[string]map[string][]*models.BuiltinPayload),
		buildPayloadsByDB:      make(map[uint64]map[string]map[string][]*models.BuiltinPayload),
	}

	bc.SyncBuiltinPayloads()
	return bc
}
func (b *BuiltinPayloadCacheType) StatChanged(total, lastUpdated int64) bool {
	if b.statTotal == total && b.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (b *BuiltinPayloadCacheType) SyncBuiltinPayloads() {
	b.initBuiltinPayloadsByFile()

	err := b.syncBuiltinPayloadsByDB()
	if err != nil {
		logger.Errorf("failed to sync builtin payload: %v", err)
	}

	go b.loopSyncBuiltinPayloadsByDB()
}

func (b *BuiltinPayloadCacheType) initBuiltinPayloadsByFile() error {
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

		// alerts
		files, err := file.FilesUnder(componentDir + "/alerts")
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

					b.addBuiltinPayloadByFile(&builtinAlert)
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

				b.addBuiltinPayloadByFile(&builtinDashboard)
			}
		} else if err != nil {
			logger.Warningf("read builtin component dash dir fail %s %v", component.Ident, err)
		}
	}

	return nil
}

func (b *BuiltinPayloadCacheType) loopSyncBuiltinPayloadsByDB() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := b.syncBuiltinPayloadsByDB(); err != nil {
			logger.Warning("failed to sync datasources:", err)
		}
	}
}

func (b *BuiltinPayloadCacheType) syncBuiltinPayloadsByDB() error {
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

	bc, err := models.BuiltinPayloadsGetAll(b.ctx)
	if err != nil {
		dumper.PutSyncRecord("builtin_payloads", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call BuiltinPayloadsGetAllMap")
	}

	b.SetBuiltinPayloadInDB(bc, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	b.stats.GaugeCronDuration.WithLabelValues("sync_builtin_payloads").Set(float64(ms))
	b.stats.GaugeSyncNumber.WithLabelValues("sync_builtin_payloads").Set(float64(len(bc)))

	logger.Infof("timer: sync builtin payloads done, cost: %dms, number: %d", ms, len(bc))
	dumper.PutSyncRecord("builtin_payloads", start.Unix(), ms, len(bc), "success")

	return nil
}

// SetBuiltinPayload sets the builtin payloads in the cache, only for payloads created by user.
func (b *BuiltinPayloadCacheType) SetBuiltinPayloadInDB(bp []*models.BuiltinPayload, total, lastUpdated int64) {
	for _, payload := range bp {
		if payload.UpdatedBy == SYSTEM {
			continue
		} else {
			b.addBuiltinPayloadByDB(payload)
		}
	}

	// only one goroutine used, so no need lock
	b.statTotal = total
	b.statLastUpdated = lastUpdated
}

func (b *BuiltinPayloadCacheType) GetBuiltinPayload(typ, cate, query string, componentId uint64) ([]*models.BuiltinPayload, error) {
	var result []*models.BuiltinPayload
	// Prepare the maps to hold the builtin payloads for each types
	var buildPayloadsInType []map[string][]*models.BuiltinPayload
	// Prepare the maps to hold the builtin payloads for each category
	var buildPayloadsInCate [][]*models.BuiltinPayload

	b.RLock()
	defer b.RUnlock()

	sources := []map[string]map[string][]*models.BuiltinPayload{
		b.buildPayloadsByFile[componentId],
		b.buildPayloadsByDB[componentId],
	}

	for _, source := range sources {
		bpInType, exist := source[typ]
		if !exist {
			continue
		}

		buildPayloadsInType = append(buildPayloadsInType, bpInType)
	}

	// Check category, if cate is empty, we will return all categories
	for _, bpInType := range buildPayloadsInType {
		if cate != "" {
			bpInCate, exists := bpInType[cate]
			if !exists {
				return nil, fmt.Errorf("no builtin payloads found for type %s and cate %s", typ, cate)
			}
			buildPayloadsInCate = append(buildPayloadsInCate, bpInCate)
		} else {
			for _, cateMap := range bpInType {
				buildPayloadsInCate = append(buildPayloadsInCate, cateMap)
			}
		}
	}

	// Check query
	for _, bpInCate := range buildPayloadsInCate {
		for _, payload := range bpInCate {
			if query != "" && !strings.Contains(payload.Name, query) && !strings.Contains(payload.Tags, query) {
				continue
			}
			result = append(result, payload)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no results found")
	}

	// Sort the result by id
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result, nil
}

// GetBuiltinPayloadByUUID returns the builtin payload by uuid
// This function is in low performance, better not to use it in high frequency.
func (b *BuiltinPayloadCacheType) GetBuiltinPayloadByUUID(uuid int64) (*models.BuiltinPayload, error) {
	b.RLock()
	defer b.RUnlock()

	for _, typeMap := range b.buildPayloadsByFile {
		for _, cateMap := range typeMap {
			for _, payloads := range cateMap {
				for _, payload := range payloads {
					if payload.UUID == uuid {
						return payload, nil
					}
				}
			}
		}
	}

	for _, typeMap := range b.buildPayloadsByDB {
		for _, cateMap := range typeMap {
			for _, payloads := range cateMap {
				for _, payload := range payloads {
					if payload.UUID == uuid {
						return payload, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("no results found")
}

// getBuiltinPayloadsByComponentId returns all builtin payloads for a given component ID
// It combines payloads from both file and database caches.
// This function is not safe, so it should be called with a lock.
func (b *BuiltinPayloadCacheType) getBuiltinPayloadsByComponentId(componentId uint64) (map[string]map[string][]*models.BuiltinPayload, error) {
	bpInCateInFile, okInFile := b.buildPayloadsByFile[componentId]
	bpInCateInDB, okInDB := b.buildPayloadsByDB[componentId]

	if !okInFile && !okInDB {
		return nil, fmt.Errorf("no builtin payloads found for component id %d", componentId)
	}

	result := make(map[string]map[string][]*models.BuiltinPayload)

	if okInFile {
		for typ, cateMap := range bpInCateInFile {
			result[typ] = cateMap
		}
	}

	// Merge the payloads from the database if they exist
	if okInDB {
		for typ, cateMap := range bpInCateInDB {
			if _, exists := result[typ]; !exists {
				result[typ] = cateMap
			} else {
				for cate, payloads := range cateMap {
					result[typ][cate] = append(result[typ][cate], payloads...)
				}
			}
		}
	}

	return result, nil
}

func (b *BuiltinPayloadCacheType) GetBuiltinPayloadCates(typ string, componentId uint64) ([]string, error) {
	b.RLock()
	defer b.RUnlock()

	var result set.StringSet

	sources := []map[string]map[string][]*models.BuiltinPayload{
		b.buildPayloadsByFile[componentId],
		b.buildPayloadsByDB[componentId],
	}

	for _, source := range sources {
		if source == nil {
			continue
		}
		typeData := source[typ]
		if typeData == nil {
			continue
		}
		for cate := range typeData {
			result.Add(cate)
		}
	}

	return result.ToSlice(), nil
}

// addBuiltinPayloadByFile adds a new builtin payload to the cache for file.
func (b *BuiltinPayloadCacheType) addBuiltinPayloadByFile(bp *models.BuiltinPayload) {
	b.Lock()
	defer b.Unlock()

	bpInType, exists := b.buildPayloadsByFile[bp.ComponentID]
	if !exists {
		bpInType = make(map[string]map[string][]*models.BuiltinPayload)
	}
	bpInCate, exists := bpInType[bp.Type]
	if !exists {
		bpInCate = make(map[string][]*models.BuiltinPayload)
	}
	bps, exists := bpInCate[bp.Cate]
	if !exists {
		bps = make([]*models.BuiltinPayload, 0)
	}
	bpInCate[bp.Cate] = append(bps, bp)
	bpInType[bp.Type] = bpInCate
	// Add key value data to bpsInSystem
	b.buildPayloadsByFile[bp.ComponentID] = bpInType
}

// addBuiltinPayloadByDB adds a new builtin payload to the cache for db.
func (b *BuiltinPayloadCacheType) addBuiltinPayloadByDB(bp *models.BuiltinPayload) {
	b.Lock()
	defer b.Unlock()

	bpInType, exists := b.buildPayloadsByDB[bp.ComponentID]
	if !exists {
		bpInType = make(map[string]map[string][]*models.BuiltinPayload)
	}
	bpInCate, exists := bpInType[bp.Type]
	if !exists {
		bpInCate = make(map[string][]*models.BuiltinPayload)
	}
	bps, exists := bpInCate[bp.Cate]
	if !exists {
		bps = make([]*models.BuiltinPayload, 0)
	}
	bpInCate[bp.Cate] = append(bps, bp)
	bpInType[bp.Type] = bpInCate
	// Add key value data to bpsInSystem
	b.buildPayloadsByDB[bp.ComponentID] = bpInType
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
