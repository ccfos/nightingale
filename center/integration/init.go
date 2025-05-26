package integration

import (
	"encoding/json"
	"path"
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

		// metrics
		files, err := file.FilesUnder(componentDir + "/metrics")
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
