package integration

import (
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
