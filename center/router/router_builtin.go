package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

// 创建 builtin_cate
func (rt *Router) builtinCateFavoriteAdd(c *gin.Context) {
	var f models.BuiltinCate
	ginx.BindJSON(c, &f)

	if f.Name == "" {
		ginx.Bomb(http.StatusBadRequest, "name is empty")
	}

	me := c.MustGet("user").(*models.User)
	f.UserId = me.Id

	ginx.NewRender(c).Message(f.Create(rt.Ctx))
}

// 删除 builtin_cate
func (rt *Router) builtinCateFavoriteDel(c *gin.Context) {
	name := ginx.UrlParamStr(c, "name")
	me := c.MustGet("user").(*models.User)

	ginx.NewRender(c).Message(models.BuiltinCateDelete(rt.Ctx, name, me.Id))
}

type Payload struct {
	Cate    string      `json:"cate"`
	Fname   string      `json:"fname"`
	Name    string      `json:"name"`
	Configs interface{} `json:"configs"`
	Tags    string      `json:"tags"`
}

type BoardCate struct {
	Name     string    `json:"name"`
	IconUrl  string    `json:"icon_url"`
	Boards   []Payload `json:"boards"`
	Favorite bool      `json:"favorite"`
}

func (rt *Router) builtinBoardDetailGets(c *gin.Context) {
	var payload Payload
	ginx.BindJSON(c, &payload)

	fp := rt.Center.BuiltinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}

	fn := fp + "/" + payload.Cate + "/dashboards/" + payload.Fname
	content, err := file.ReadBytes(fn)
	ginx.Dangerous(err)

	err = json.Unmarshal(content, &payload)
	ginx.NewRender(c).Data(payload, err)
}

func (rt *Router) builtinBoardCateGets(c *gin.Context) {
	fp := rt.Center.BuiltinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}

	me := c.MustGet("user").(*models.User)
	buildinFavoritesMap, err := models.BuiltinCateGetByUserId(rt.Ctx, me.Id)
	if err != nil {
		logger.Warningf("get builtin favorites fail: %v", err)
	}

	var boardCates []BoardCate
	dirList, err := file.DirsUnder(fp)
	ginx.Dangerous(err)
	for _, dir := range dirList {
		var boardCate BoardCate
		boardCate.Name = dir
		files, err := file.FilesUnder(fp + "/" + dir + "/dashboards")
		ginx.Dangerous(err)
		if len(files) == 0 {
			continue
		}

		var boards []Payload
		for _, f := range files {
			fn := fp + "/" + dir + "/dashboards/" + f
			content, err := file.ReadBytes(fn)
			if err != nil {
				logger.Warningf("add board fail: %v", err)
				continue
			}

			var payload Payload
			err = json.Unmarshal(content, &payload)
			if err != nil {
				logger.Warningf("add board:%s fail: %v", fn, err)
				continue
			}
			payload.Cate = dir
			payload.Fname = f
			payload.Configs = ""
			boards = append(boards, payload)
		}
		boardCate.Boards = boards

		if _, ok := buildinFavoritesMap[dir]; ok {
			boardCate.Favorite = true
		}

		iconFiles, _ := file.FilesUnder(fp + "/" + dir + "/icon")
		if len(iconFiles) > 0 {
			boardCate.IconUrl = fmt.Sprintf("/api/n9e/integrations/icon/%s/%s", dir, iconFiles[0])
		}

		boardCates = append(boardCates, boardCate)
	}
	ginx.NewRender(c).Data(boardCates, nil)
}

func (rt *Router) builtinBoardGets(c *gin.Context) {
	fp := rt.Center.BuiltinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}

	var fileList []string
	dirList, err := file.DirsUnder(fp)
	ginx.Dangerous(err)
	for _, dir := range dirList {
		files, err := file.FilesUnder(fp + "/" + dir + "/dashboards")
		ginx.Dangerous(err)
		fileList = append(fileList, files...)
	}

	names := make([]string, 0, len(fileList))
	for _, f := range fileList {
		if !strings.HasSuffix(f, ".json") {
			continue
		}

		name := strings.TrimSuffix(f, ".json")
		names = append(names, name)
	}

	ginx.NewRender(c).Data(names, nil)
}

type AlertCate struct {
	Name       string             `json:"name"`
	IconUrl    string             `json:"icon_url"`
	AlertRules []models.AlertRule `json:"alert_rules"`
	Favorite   bool               `json:"favorite"`
}

func (rt *Router) builtinAlertCateGets(c *gin.Context) {
	fp := rt.Center.BuiltinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}

	me := c.MustGet("user").(*models.User)
	buildinFavoritesMap, err := models.BuiltinCateGetByUserId(rt.Ctx, me.Id)
	if err != nil {
		logger.Warningf("get builtin favorites fail: %v", err)
	}

	var alertCates []AlertCate
	dirList, err := file.DirsUnder(fp)
	ginx.Dangerous(err)
	for _, dir := range dirList {
		var alertCate AlertCate
		alertCate.Name = dir
		files, err := file.FilesUnder(fp + "/" + dir + "/alerts")
		ginx.Dangerous(err)

		var alertRules []models.AlertRule
		for _, f := range files {
			fn := fp + "/" + dir + "/alerts/" + f
			content, err := file.ReadBytes(fn)
			if err != nil {
				logger.Warningf("add board fail: %v", err)
				continue
			}

			var ars []models.AlertRule
			err = json.Unmarshal(content, &ars)
			if err != nil {
				logger.Warningf("add board:%s fail: %v", fn, err)
				continue
			}
			alertRules = append(alertRules, ars...)
		}
		alertCate.AlertRules = alertRules
		iconFiles, _ := file.FilesUnder(fp + "/" + dir + "/icon")
		if len(iconFiles) > 0 {
			alertCate.IconUrl = fmt.Sprintf("/api/n9e/integrations/icon/%s/%s", dir, iconFiles[0])
		}

		if _, ok := buildinFavoritesMap[dir]; ok {
			alertCate.Favorite = true
		}

		alertCates = append(alertCates, alertCate)
	}
	ginx.NewRender(c).Data(alertCates, nil)
}

type builtinAlertRulesList struct {
	Name       string                        `json:"name"`
	IconUrl    string                        `json:"icon_url"`
	AlertRules map[string][]models.AlertRule `json:"alert_rules"`
	Favorite   bool                          `json:"favorite"`
}

func (rt *Router) builtinAlertRules(c *gin.Context) {
	fp := rt.Center.BuiltinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}

	me := c.MustGet("user").(*models.User)
	buildinFavoritesMap, err := models.BuiltinCateGetByUserId(rt.Ctx, me.Id)
	if err != nil {
		logger.Warningf("get builtin favorites fail: %v", err)
	}

	var alertCates []builtinAlertRulesList
	dirList, err := file.DirsUnder(fp)
	ginx.Dangerous(err)
	for _, dir := range dirList {
		var alertCate builtinAlertRulesList
		alertCate.Name = dir
		files, err := file.FilesUnder(fp + "/" + dir + "/alerts")
		ginx.Dangerous(err)
		if len(files) == 0 {
			continue
		}

		alertRules := make(map[string][]models.AlertRule)
		for _, f := range files {
			fn := fp + "/" + dir + "/alerts/" + f
			content, err := file.ReadBytes(fn)
			if err != nil {
				logger.Warningf("add board fail: %v", err)
				continue
			}

			var ars []models.AlertRule
			err = json.Unmarshal(content, &ars)
			if err != nil {
				logger.Warningf("add board:%s fail: %v", fn, err)
				continue
			}
			alertRules[strings.TrimSuffix(f, ".json")] = ars
		}

		alertCate.AlertRules = alertRules
		iconFiles, _ := file.FilesUnder(fp + "/" + dir + "/icon")
		if len(iconFiles) > 0 {
			alertCate.IconUrl = fmt.Sprintf("/api/n9e/integrations/icon/%s/%s", dir, iconFiles[0])
		}

		if _, ok := buildinFavoritesMap[dir]; ok {
			alertCate.Favorite = true
		}

		alertCates = append(alertCates, alertCate)
	}
	ginx.NewRender(c).Data(alertCates, nil)
}

// read the json file content
func (rt *Router) builtinBoardGet(c *gin.Context) {
	name := ginx.UrlParamStr(c, "name")
	dirpath := rt.Center.BuiltinIntegrationsDir
	if dirpath == "" {
		dirpath = path.Join(runner.Cwd, "integrations")
	}

	dirList, err := file.DirsUnder(dirpath)
	ginx.Dangerous(err)
	for _, dir := range dirList {
		jsonFile := dirpath + "/" + dir + "/dashboards/" + name + ".json"
		if file.IsExist(jsonFile) {
			body, err := file.ReadString(jsonFile)
			ginx.NewRender(c).Data(body, err)
			return
		}
	}

	ginx.Bomb(http.StatusBadRequest, "%s not found", name)
}

func (rt *Router) builtinIcon(c *gin.Context) {
	fp := rt.Center.BuiltinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}

	cate := ginx.UrlParamStr(c, "cate")
	iconPath := fp + "/" + cate + "/icon/" + ginx.UrlParamStr(c, "name")
	c.File(path.Join(iconPath))
}

func (rt *Router) builtinMarkdown(c *gin.Context) {
	fp := rt.Center.BuiltinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}
	cate := ginx.UrlParamStr(c, "cate")

	var markdown []byte
	markdownDir := fp + "/" + cate + "/markdown"
	markdownFiles, err := file.FilesUnder(markdownDir)
	if err != nil {
		logger.Warningf("get markdown fail: %v", err)
	} else if len(markdownFiles) > 0 {
		f := markdownFiles[0]
		fn := markdownDir + "/" + f
		markdown, err = file.ReadBytes(fn)
		if err != nil {
			logger.Warningf("get collect fail: %v", err)
		}
	}
	ginx.NewRender(c).Data(string(markdown), nil)
}
