package integration

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

func Init(ctx *ctx.Context, builtinIntegrationsDir string) {
	fp := builtinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}

	// var fileList []string
	dirList, err := file.DirsUnder(fp)
	ginx.Dangerous(err)

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
		} else {
			logger.Warning("no logo found for builtin component", component.Ident)
		}

		// get description
		files, err = file.FilesUnder(componentDir + "/markdown")
		if err == nil && len(files) > 0 {
			var readmeFile string
			for _, file := range files {
				if strings.HasSuffix(strings.ToLower(file), "md") {
					readmeFile = file
				}
			}
			if readmeFile != "" {
				component.Readme, _ = file.ReadString(readmeFile)
			}
		} else {
			logger.Warning("no markdown found for builtin component", component.Ident)
		}

		exists, _ := models.BuiltinComponentExists(ctx, &component)
		if !exists {
			err = component.Add(ctx, "system")
			if err != nil {
				logger.Warning("add builtin component fail", component, err)
				continue
			}
		}

		// alerts
		files, err = file.FilesUnder(componentDir + "/alerts")
		if err == nil && len(files) > 0 {
			for _, f := range files {
				bs, err := file.ReadBytes(f)
				if err != nil {
					logger.Warning("read builtin component alerts file fail", f, err)
					continue
				}

				alerts := []models.AlertRule{}
				err = json.Unmarshal(bs, &alerts)
				if err != nil {
					logger.Warning("parse builtin component alerts file fail", f, err)
					continue
				}

				for _, alert := range alerts {
					content, err := json.Marshal(alert)
					if err != nil {
						logger.Warning("marshal builtin alert fail", alert, err)
						continue
					}

					builtinAlert := models.BuiltinPayload{
						Component: component.Ident,
						Type:      "alert",
						Cate:      f,
						Name:      alert.Name,
						Content:   string(content),
					}

					exists, err := models.BuiltinPayloadExists(ctx, &builtinAlert)
					if err != nil {
						logger.Warning("check builtin alert exists fail", builtinAlert, err)
						continue
					}
					if exists {
						continue
					}

					err = builtinAlert.Add(ctx, "system")
					if err != nil {
						logger.Warning("add builtin alert fail", builtinAlert, err)
						continue
					}
				}
			}
		} else {
			logger.Warning("no alerts found for builtin component", component.Ident)
		}

		// dashboards
		files, err = file.FilesUnder(componentDir + "/dashboards")
		if err == nil && len(files) > 0 {
			for _, f := range files {
				bs, err := file.ReadBytes(f)
				if err != nil {
					logger.Warning("read builtin component dashboards file fail", f, err)
					continue
				}

				dashboards := []models.Dashboard{}
				err = json.Unmarshal(bs, &dashboards)
				if err != nil {
					logger.Warning("parse builtin component dashboards file fail", f, err)
					continue
				}

				for _, dashboard := range dashboards {
					content, err := json.Marshal(dashboard)
					if err != nil {
						logger.Warning("marshal builtin dashboard fail", dashboard, err)
						continue
					}

					builtinDashboard := models.BuiltinPayload{
						Component: component.Ident,
						Type:      "dashboard",
						Cate:      "",
						Name:      dashboard.Name,
						Content:   string(content),
					}

					exists, err := models.BuiltinPayloadExists(ctx, &builtinDashboard)
					if err != nil {
						logger.Warning("check builtin dashboard exists fail", builtinDashboard, err)
						continue
					}

					if exists {
						continue
					}

					err = builtinDashboard.Add(ctx, "system")
					if err != nil {
						logger.Warning("add builtin dashboard fail", builtinDashboard, err)
						continue
					}
				}
			}
		} else {
			logger.Warning("no dashboards found for builtin component", component.Ident)
		}

		// metrics
		files, err = file.FilesUnder(componentDir + "/metrics")
		if err == nil && len(files) > 0 {
			for _, f := range files {
				bs, err := file.ReadBytes(f)
				if err != nil {
					logger.Warning("read builtin component metrics file fail", f, err)
					continue
				}

				metrics := []models.BuiltinMetric{}
				err = json.Unmarshal(bs, &metrics)
				if err != nil {
					logger.Warning("parse builtin component metrics file fail", f, err)
					continue
				}

				for _, metric := range metrics {
					exists, err := models.BuiltinMetricExists(ctx, &metric)
					if err != nil {
						logger.Warning("check builtin metric exists fail", metric, err)
						continue
					}
					if exists {
						continue
					}
					err = metric.Add(ctx, "system")
					if err != nil {
						logger.Warning("add builtin metric fail", metric, err)
						continue
					}
				}
			}
		} else {
			logger.Warning("no metrics found for builtin component", component.Ident)
		}
	}
}
