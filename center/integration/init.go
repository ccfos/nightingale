package integration

import (
	"encoding/json"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

const SYSTEM = "system"

var BuiltinPayloadInFile *BuiltinPayloadInFileType

type BuiltinPayloadInFileType struct {
	Data      map[uint64]map[string]map[string][]*models.BuiltinPayload // map[componet_id]map[type]map[cate][]*models.BuiltinPayload
	IndexData map[int64]*models.BuiltinPayload                          // map[uuid]payload
}

func Init(ctx *ctx.Context, builtinIntegrationsDir string) {
	BuiltinPayloadInFile = NewBuiltinPayloadInFileType()

	err := models.InitBuiltinPayloads(ctx)
	if err != nil {
		logger.Warning("init old builtinPayloads fail ", err)
		return
	}

	if res, err := models.ConfigsSelectByCkey(ctx, "disable_integration_init"); err != nil {
		logger.Error("fail to get value 'disable_integration_init' from configs", err)
		return
	} else if len(res) != 0 {
		logger.Info("disable_integration_init is set, skip integration init")
		return
	}

	fp := builtinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}

	// var fileList []string
	dirList, err := file.DirsUnder(fp)
	if err != nil {
		logger.Warning("read builtin component dir fail ", err)
		return
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

		exists, _ := models.BuiltinComponentExists(ctx, &component)
		if !exists {
			err = component.Add(ctx, SYSTEM)
			if err != nil {
				logger.Warning("add builtin component fail ", component, err)
				continue
			}
		} else {
			old, err := models.BuiltinComponentGet(ctx, "ident = ?", component.Ident)
			if err != nil {
				logger.Warning("get builtin component fail ", component, err)
				continue
			}

			if old == nil {
				logger.Warning("get builtin component nil ", component)
				continue
			}

			if old.UpdatedBy == SYSTEM {
				now := time.Now().Unix()
				old.CreatedAt = now
				old.UpdatedAt = now
				old.Readme = component.Readme
				old.UpdatedBy = SYSTEM

				err = models.DB(ctx).Model(old).Select("*").Updates(old).Error
				if err != nil {
					logger.Warning("update builtin component fail ", old, err)
				}
			}
			component.ID = old.ID
		}

		// delete uuid is emtpy
		err = models.DB(ctx).Exec("delete from builtin_payloads where uuid = 0 and type != 'collect' and (updated_by = 'system' or updated_by = '')").Error
		if err != nil {
			logger.Warning("delete builtin payloads fail ", err)
		}

		// delete builtin metrics uuid is emtpy
		err = models.DB(ctx).Exec("delete from builtin_metrics where uuid = 0 and (updated_by = 'system' or updated_by = '')").Error
		if err != nil {
			logger.Warning("delete builtin metrics fail ", err)
		}

		// 删除 uuid%1000 不为 0 uuid > 1000000000000000000 且 type 为 dashboard 的记录
		err = models.DB(ctx).Exec("delete from builtin_payloads where uuid%1000 != 0 and uuid > 1000000000000000000 and type = 'dashboard' and updated_by = 'system'").Error
		if err != nil {
			logger.Warning("delete builtin payloads fail ", err)
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

				newAlerts := []models.AlertRule{}
				for _, alert := range alerts {
					if alert.UUID == 0 {
						alert.UUID = time.Now().UnixNano()
					}

					newAlerts = append(newAlerts, alert)
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
					BuiltinPayloadInFile.addBuiltinPayload(&builtinAlert)

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
					// 补全文件中的 uuid
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
				BuiltinPayloadInFile.addBuiltinPayload(&builtinDashboard)
			}
		} else if err != nil {
			logger.Warningf("read builtin component dash dir fail %s %v", component.Ident, err)
		}

		// metrics
		// files, err = file.FilesUnder(componentDir + "/metrics")
		// if err == nil && len(files) > 0 {
		// 	for _, f := range files {
		// 		fp := componentDir + "/metrics/" + f
		// 		bs, err := file.ReadBytes(fp)
		// 		if err != nil {
		// 			logger.Warning("read builtin component metrics file fail", f, err)
		// 			continue
		// 		}

		// 		metrics := []models.BuiltinMetric{}
		// 		newMetrics := []models.BuiltinMetric{}
		// 		err = json.Unmarshal(bs, &metrics)
		// 		if err != nil {
		// 			logger.Warning("parse builtin component metrics file fail", f, err)
		// 			continue
		// 		}

		// 		for _, metric := range metrics {
		// 			if metric.UUID == 0 {
		// 				metric.UUID = time.Now().UnixNano()
		// 			}
		// 			newMetrics = append(newMetrics, metric)

		// 			old, err := models.BuiltinMetricGet(ctx, "uuid = ?", metric.UUID)
		// 			if err != nil {
		// 				logger.Warning("get builtin metrics fail ", metric, err)
		// 				continue
		// 			}

		// 			if old == nil {
		// 				err := metric.Add(ctx, SYSTEM)
		// 				if err != nil {
		// 					logger.Warning("add builtin metrics fail ", metric, err)
		// 				}
		// 				continue
		// 			}

		// 			if old.UpdatedBy == SYSTEM {
		// 				old.Collector = metric.Collector
		// 				old.Typ = metric.Typ
		// 				old.Name = metric.Name
		// 				old.Unit = metric.Unit
		// 				old.Note = metric.Note
		// 				old.Lang = metric.Lang
		// 				old.Expression = metric.Expression

		// 				err = models.DB(ctx).Model(old).Select("*").Updates(old).Error
		// 				if err != nil {
		// 					logger.Warningf("update builtin metric:%+v fail %v", metric, err)
		// 				}
		// 			}
		// 		}
		// 	}
		// } else if err != nil {
		// 	logger.Warningf("read builtin component metrics dir fail %s %v", component.Ident, err)
		// }
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

func NewBuiltinPayloadInFileType() *BuiltinPayloadInFileType {
	return &BuiltinPayloadInFileType{
		Data:      make(map[uint64]map[string]map[string][]*models.BuiltinPayload),
		IndexData: make(map[int64]*models.BuiltinPayload),
	}
}

func (b *BuiltinPayloadInFileType) addBuiltinPayload(bp *models.BuiltinPayload) {
	if _, exists := b.Data[bp.ComponentID]; !exists {
		b.Data[bp.ComponentID] = make(map[string]map[string][]*models.BuiltinPayload)
	}
	bpInType := b.Data[bp.ComponentID]
	if _, exists := bpInType[bp.Type]; !exists {
		bpInType[bp.Type] = make(map[string][]*models.BuiltinPayload)
	}
	bpInCate := bpInType[bp.Type]
	if _, exists := bpInCate[bp.Cate]; !exists {
		bpInCate[bp.Cate] = make([]*models.BuiltinPayload, 0)
	}
	bpInCate[bp.Cate] = append(bpInCate[bp.Cate], bp)

	b.IndexData[bp.UUID] = bp
}

func (b *BuiltinPayloadInFileType) GetBuiltinPayload(typ, cate, query string, componentId uint64) ([]*models.BuiltinPayload, error) {

	var result []*models.BuiltinPayload
	source := b.Data[componentId]

	if source == nil {
		return nil, nil
	}

	typeMap, exists := source[typ]
	if !exists {
		return nil, nil
	}

	if cate != "" {
		payloads, exists := typeMap[cate]
		if !exists {
			return nil, nil
		}
		result = append(result, filterByQuery(payloads, query)...)
	} else {
		for _, payloads := range typeMap {
			result = append(result, filterByQuery(payloads, query)...)
		}
	}

	if len(result) > 0 {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Name < result[j].Name
		})
	}

	return result, nil
}

func (b *BuiltinPayloadInFileType) GetBuiltinPayloadCates(typ string, componentId uint64) ([]string, error) {
	var result []string
	source := b.Data[componentId]
	if source == nil {
		return result, nil
	}

	typeData := source[typ]
	if typeData == nil {
		return result, nil
	}
	for cate := range typeData {
		result = append(result, cate)
	}

	sort.Strings(result)
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
