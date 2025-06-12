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
	buildPayloadsByFile          map[uint64]map[string]map[string][]*models.BuiltinPayload // map[componet_id]map[type]map[cate][]*models.BuiltinPayload
	buildPayloadsByFileUUIDIndex map[int64]*models.BuiltinPayload                          // map[uuid]payload
	// Created from db, need to be synced with the database
	buildPayloadsByDB          map[uint64]map[string]map[string][]*models.BuiltinPayload // map[componet_id]map[type]map[cate][]*models.BuiltinPayload
	buildPayloadsByDBUUIDIndex map[int64]*models.BuiltinPayload                          // map[uuid]payload
}

func NewBuiltinPayloadCache(ctx *ctx.Context, stats *Stats, builtinIntegrationsDir string) *BuiltinPayloadCacheType {
	bc := &BuiltinPayloadCacheType{
		statTotal:                    -1,
		statLastUpdated:              -1,
		ctx:                          ctx,
		stats:                        stats,
		builtinIntegrationsDir:       builtinIntegrationsDir,
		buildPayloadsByFile:          make(map[uint64]map[string]map[string][]*models.BuiltinPayload),
		buildPayloadsByFileUUIDIndex: make(map[int64]*models.BuiltinPayload),
		buildPayloadsByDB:            make(map[uint64]map[string]map[string][]*models.BuiltinPayload),
		buildPayloadsByDBUUIDIndex:   make(map[int64]*models.BuiltinPayload),
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
	b.Lock()
	defer b.Unlock()

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
	b.Lock()
	defer b.Unlock()

	// Clear the old cache, wait for the next sync to rebuild it.
	b.clearbuildPayloadsByDBAndIndex()

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

func (b *BuiltinPayloadCacheType) clearbuildPayloadsByDBAndIndex() {
	b.buildPayloadsByDB = make(map[uint64]map[string]map[string][]*models.BuiltinPayload)
	b.buildPayloadsByDBUUIDIndex = make(map[int64]*models.BuiltinPayload)
}

func (b *BuiltinPayloadCacheType) GetBuiltinPayload(typ, cate, query string, componentId uint64) ([]*models.BuiltinPayload, error) {
	b.RLock()
	defer b.RUnlock()

	sources := []map[string]map[string][]*models.BuiltinPayload{
		b.buildPayloadsByFile[componentId],
		b.buildPayloadsByDB[componentId],
	}

	var result []*models.BuiltinPayload

	for _, source := range sources {
		if source == nil {
			continue
		}

		typeMap, exists := source[typ]
		if !exists {
			continue
		}

		if cate != "" {
			payloads, exists := typeMap[cate]
			if !exists {
				continue
			}
			result = append(result, filterByQuery(payloads, query)...)
		} else {
			for _, payloads := range typeMap {
				result = append(result, filterByQuery(payloads, query)...)
			}
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no builtin payloads found for type=%s cate=%s query=%s", typ, cate, query)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result, nil
}

func filterByQuery(payloads []*models.BuiltinPayload, query string) []*models.BuiltinPayload {
	if query == "" {
		return payloads
	}

	var filtered []*models.BuiltinPayload
	for _, p := range payloads {
		if strings.Contains(p.Name, query) || strings.Contains(p.Tags, query) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// GetBuiltinPayloadByUUID returns the builtin payload by uuid
// This function is in low performance, better not to use it in high frequency.
func (b *BuiltinPayloadCacheType) GetBuiltinPayloadByUUID(uuid int64) (*models.BuiltinPayload, error) {
	b.RLock()
	defer b.RUnlock()

	if payload, exists := b.buildPayloadsByFileUUIDIndex[uuid]; exists {
		return payload, nil
	}

	if payload, exists := b.buildPayloadsByDBUUIDIndex[uuid]; exists {
		return payload, nil
	}

	return nil, fmt.Errorf("no results found for uuid=%d", uuid)
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
	b.addBuiltinPayload(b.buildPayloadsByFile, b.buildPayloadsByFileUUIDIndex, bp)
}

// addBuiltinPayloadByDB adds a new builtin payload to the cache for db.
func (b *BuiltinPayloadCacheType) addBuiltinPayloadByDB(bp *models.BuiltinPayload) {
	b.addBuiltinPayload(b.buildPayloadsByDB, b.buildPayloadsByDBUUIDIndex, bp)
}

// addBuiltinPayload
func (b *BuiltinPayloadCacheType) addBuiltinPayload(
	cacheMap map[uint64]map[string]map[string][]*models.BuiltinPayload,
	indexMap map[int64]*models.BuiltinPayload,
	bp *models.BuiltinPayload,
) {
	if _, exists := cacheMap[bp.ComponentID]; !exists {
		cacheMap[bp.ComponentID] = make(map[string]map[string][]*models.BuiltinPayload)
	}
	bpInType := cacheMap[bp.ComponentID]
	if _, exists := bpInType[bp.Type]; !exists {
		bpInType[bp.Type] = make(map[string][]*models.BuiltinPayload)
	}
	bpInCate := bpInType[bp.Type]
	if _, exists := bpInCate[bp.Cate]; !exists {
		bpInCate[bp.Cate] = make([]*models.BuiltinPayload, 0)
	}
	bpInCate[bp.Cate] = append(bpInCate[bp.Cate], bp)

	indexMap[bp.UUID] = bp
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
