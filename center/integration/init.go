package integration

import (
	"encoding/json"
	"path"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

const SYSTEM = "system"

func Init(ctx *ctx.Context, builtinIntegrationsDir string) {
	err := models.InitBuiltinPayloads(ctx)
	if err != nil {
		logger.Warning("init old builtinPayloads fail ", err)
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
				writeAlertFileFlag := false
				for _, alert := range alerts {
					if alert.UUID == 0 {
						writeAlertFileFlag = true
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

					old, err := models.BuiltinPayloadGet(ctx, "uuid = ?", alert.UUID)
					if err != nil {
						logger.Warning("get builtin alert fail ", builtinAlert, err)
						continue
					}

					if old == nil {
						err := builtinAlert.Add(ctx, SYSTEM)
						if err != nil {
							logger.Warning("add builtin alert fail ", builtinAlert, err)
						}
						continue
					}

					if old.UpdatedBy == SYSTEM {
						old.ComponentID = component.ID
						old.Content = string(content)
						old.Name = alert.Name
						old.Tags = alert.AppendTags
						err = models.DB(ctx).Model(old).Select("*").Updates(old).Error
						if err != nil {
							logger.Warningf("update builtin alert:%+v fail %v", builtinAlert, err)
						}
					}
				}

				if writeAlertFileFlag {
					bs, err = json.MarshalIndent(newAlerts, "", "    ")
					if err != nil {
						logger.Warning("marshal builtin alerts fail ", newAlerts, err)
						continue
					}

					_, err = file.WriteBytes(fp, bs)
					if err != nil {
						logger.Warning("write builtin alerts file fail ", f, err)
					}
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
					dashboard.UUID = time.Now().UnixNano()
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

				old, err := models.BuiltinPayloadGet(ctx, "uuid = ?", dashboard.UUID)
				if err != nil {
					logger.Warning("get builtin alert fail ", builtinDashboard, err)
					continue
				}

				if old == nil {
					err := builtinDashboard.Add(ctx, SYSTEM)
					if err != nil {
						logger.Warning("add builtin alert fail ", builtinDashboard, err)
					}
					continue
				}

				if old.UpdatedBy == SYSTEM {
					old.ComponentID = component.ID
					old.Content = string(content)
					old.Name = dashboard.Name
					old.Tags = dashboard.Tags
					err = models.DB(ctx).Model(old).Select("*").Updates(old).Error
					if err != nil {
						logger.Warningf("update builtin alert:%+v fail %v", builtinDashboard, err)
					}
				}
			}
		} else if err != nil {
			logger.Warningf("read builtin component dash dir fail %s %v", component.Ident, err)
		}

		// metrics
		files, err = file.FilesUnder(componentDir + "/metrics")
		if err == nil && len(files) > 0 {
			for _, f := range files {
				fp := componentDir + "/metrics/" + f
				bs, err := file.ReadBytes(fp)
				if err != nil {
					logger.Warning("read builtin component metrics file fail", f, err)
					continue
				}

				metrics := []models.BuiltinMetric{}
				newMetrics := []models.BuiltinMetric{}
				err = json.Unmarshal(bs, &metrics)
				if err != nil {
					logger.Warning("parse builtin component metrics file fail", f, err)
					continue
				}

				writeMetricFileFlag := false
				for _, metric := range metrics {
					if metric.UUID == 0 {
						writeMetricFileFlag = true
						metric.UUID = time.Now().UnixNano()
					}
					newMetrics = append(newMetrics, metric)

					old, err := models.BuiltinMetricGet(ctx, "uuid = ?", metric.UUID)
					if err != nil {
						logger.Warning("get builtin metrics fail ", metric, err)
						continue
					}

					if old == nil {
						err := metric.Add(ctx, SYSTEM)
						if err != nil {
							logger.Warning("add builtin metrics fail ", metric, err)
						}
						continue
					}

					if old.UpdatedBy == SYSTEM {
						old.Collector = metric.Collector
						old.Typ = metric.Typ
						old.Name = metric.Name
						old.Unit = metric.Unit
						old.Note = metric.Note
						old.Lang = metric.Lang
						old.Expression = metric.Expression

						err = models.DB(ctx).Model(old).Select("*").Updates(old).Error
						if err != nil {
							logger.Warningf("update builtin metric:%+v fail %v", metric, err)
						}
					}
				}

				if writeMetricFileFlag {
					bs, err = json.MarshalIndent(newMetrics, "", "    ")
					if err != nil {
						logger.Warning("marshal builtin metrics fail ", newMetrics, err)
						continue
					}

					_, err = file.WriteBytes(fp, bs)
					if err != nil {
						logger.Warning("write builtin metrics file fail ", f, err)
					}
				}

			}
		} else if err != nil {
			logger.Warningf("read builtin component metrics dir fail %s %v", component.Ident, err)
		}
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
