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

	bc, err := models.BuiltinPayloadsGetAllMap(b.ctx)
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
func (b *BuiltinPayloadCacheType) SetBuiltinPayloadInDB(bp map[int64]*models.BuiltinPayload, total, lastUpdated int64) {
	for _, payload := range bp {
		if payload.CreatedBy == SYSTEM {
			continue
		} else {
			b.addBuiltinPayload(payload, false)
		}
	}

	// only one goroutine used, so no need lock
	b.statTotal = total
	b.statLastUpdated = lastUpdated
}

func (b *BuiltinPayloadCacheType) GetBuiltinPayload(typ, cate, query string, componentId uint64) ([]*models.BuiltinPayload, error) {
	var result []*models.BuiltinPayload
	// Prepare the maps to hold the builtin payloads for each component
	var buildPayloadsInComponent map[string]map[string][]*models.BuiltinPayload
	// Prepare the maps to hold the builtin payloads for each types
	var buildPayloadsInType []map[string][]*models.BuiltinPayload
	// Prepare the maps to hold the builtin payloads for each category
	var buildPayloadsInCate [][]*models.BuiltinPayload

	b.RLock()
	defer b.RUnlock()

	// map[componet_id]map[type]map[cate][]*models.BuiltinPayload
	buildPayloadsByFile, okInBuildPayloadsByFile := b.buildPayloadsByFile[componentId]
	buildPayloadsByDB, okInBuildPayloadsByDB := b.buildPayloadsByDB[componentId]
	if okInBuildPayloadsByFile {
		buildPayloadsInComponent = buildPayloadsByFile
	} else if okInBuildPayloadsByDB {
		buildPayloadsInComponent = buildPayloadsByDB
	} else {
		return nil, fmt.Errorf("no builtin payloads found for component id %d", componentId)
	}

	// Check type
	if typ != "" {
		bpInType, exists := buildPayloadsInComponent[typ]
		if !exists {
			return nil, fmt.Errorf("no builtin payloads found for type %s", typ)
		}
		buildPayloadsInType = append(buildPayloadsInType, bpInType)
	} else {
		for _, typeMap := range buildPayloadsByDB {
			buildPayloadsInType = append(buildPayloadsInType, typeMap)
		}
	}

	// Check category
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

func (b *BuiltinPayloadCacheType) GetBuiltinPayloadById(id int64) (*models.BuiltinPayload, error) {
	b.RLock()
	defer b.RUnlock()

	for _, typeMap := range b.buildPayloadsByFile {
		for _, cateMap := range typeMap {
			for _, payloads := range cateMap {
				for _, payload := range payloads {
					if payload.ID == id {
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
					if payload.ID == id {
						return payload, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("no results found")
}

func (b *BuiltinPayloadCacheType) GetBuiltinPayloadCates(typ string, componentId uint64) ([]string, error) {
	var result set.StringSet

	bpInCateInFile, okInFile := b.buildPayloadsByFile[componentId]
	bpInCateInDB, okInDB := b.buildPayloadsByDB[componentId]

	if !okInFile && !okInDB {
		return nil, fmt.Errorf("no builtin payloads found for component id %d", componentId)
	}

	if okInFile {
		if typ != "" {
			for _, bpsInType := range bpInCateInFile {
				for cate := range bpsInType {
					result.Add(cate)
				}
			}
		} else {
			bpInType, exists := bpInCateInFile[typ]
			if exists {
				for cate := range bpInType {
					result.Add(cate)
				}
			}
		}
	}

	if okInDB {
		if typ != "" {
			for _, bpsInType := range bpInCateInDB {
				for cate := range bpsInType {
					result.Add(cate)
				}
			}
		} else {
			bpInType, exists := bpInCateInFile[typ]
			if exists {
				for cate := range bpInType {
					result.Add(cate)
				}
			}
		}
	}

	resultStrings := result.ToSlice()
	if len(resultStrings) == 0 {
		return nil, fmt.Errorf("no results found")
	}

	return resultStrings, nil
}

// addBuiltinPayload adds a new builtin payload to the cache.
// If the payload is created by the system, it is stored in bpInSystem.
// If it is created by the user, it is stored in bpInUser.
// This function will not check duplicates.
func (b *BuiltinPayloadCacheType) addBuiltinPayload(bp *models.BuiltinPayload, isSystem bool) {
	b.Lock()
	defer b.Unlock()

	if isSystem {
		bpInType, exists := b.buildPayloadsByFile[bp.ComponentID]
		if !exists {
			bpInType := make(map[string]map[string][]*models.BuiltinPayload)
			bpInType[bp.Cate] = make(map[string][]*models.BuiltinPayload)
			bpInType[bp.Cate][bp.Type] = append(bpInType[bp.Cate][bp.Type], bp)
			b.buildPayloadsByFile[bp.ComponentID] = bpInType
			return
		}
		bpInCate, exists := bpInType[bp.Type]
		if !exists {
			bpInCate = make(map[string][]*models.BuiltinPayload)
			bpInType[bp.Type] = bpInCate
			b.buildPayloadsByFile[bp.ComponentID] = bpInType
			return
		}
		bps, exists := bpInCate[bp.Cate]
		if !exists {
			bps = []*models.BuiltinPayload{bp}
			bpInCate[bp.Cate] = bps
			bpInType[bp.Cate] = bpInCate
			b.buildPayloadsByFile[bp.ComponentID] = bpInType
			return
		}
		bpInCate[bp.Cate] = append(bps, bp)
		bpInType[bp.Type] = bpInCate
		// Add key value data to bpsInSystem
		b.buildPayloadsByFile[bp.ComponentID] = bpInType
	} else {
		bpInType, exists := b.buildPayloadsByDB[bp.ComponentID]
		if !exists {
			bpInType := make(map[string]map[string][]*models.BuiltinPayload)
			bpInType[bp.Cate] = make(map[string][]*models.BuiltinPayload)
			bpInType[bp.Cate][bp.Type] = append(bpInType[bp.Cate][bp.Type], bp)
			b.buildPayloadsByDB[bp.ComponentID] = bpInType
			return
		}
		bpInCate, exists := bpInType[bp.Type]
		if !exists {
			bpInCate = make(map[string][]*models.BuiltinPayload)
			bpInType[bp.Type] = bpInCate
			b.buildPayloadsByDB[bp.ComponentID] = bpInType
			return
		}
		bps, exists := bpInCate[bp.Cate]
		if !exists {
			bps = []*models.BuiltinPayload{bp}
			bpInCate[bp.Cate] = bps
			bpInType[bp.Cate] = bpInCate
			b.buildPayloadsByDB[bp.ComponentID] = bpInType
			return
		}
		bpInCate[bp.Cate] = append(bps, bp)
		bpInType[bp.Type] = bpInCate
		// Add key value data to bpsInSystem
		b.buildPayloadsByDB[bp.ComponentID] = bpInType
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
