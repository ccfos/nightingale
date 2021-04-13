package http

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/didi/nightingale/v4/src/common/compress"
	"github.com/didi/nightingale/v4/src/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/sys"
)

var MIBS string

func mibPost(c *gin.Context) {
	MIBS = "./mibs/current_mib"
	file.EnsureDir(MIBS)
	username := loginUsername(c)
	can, err := models.UsernameCandoGlobalOp(username, "nems_network_ops")
	dangerous(err)
	if !can {
		bomb("no privilege")
	}

	module := c.PostForm("module")
	if module == "" {
		bomb("module is blank")
	}

	formFile, err := c.FormFile("file")
	dangerous(err)

	if !strings.HasSuffix(formFile.Filename, ".tar.gz") {
		bomb("file suffix only support .tar.gz")
	}

	dirOfFile := "./mibs"
	file.EnsureDir(dirOfFile)

	pathOfFile := dirOfFile + "/" + formFile.Filename
	if err := c.SaveUploadedFile(formFile, pathOfFile); err != nil {
		bomb("upload file err: %v", err)
	}

	err = compress.UnCompress(pathOfFile, MIBS)
	dangerous(err)

	absPath, err := file.RealPath(MIBS)
	dangerous(err)

	out, err, isTimeout := sys.CmdRunT(5*time.Second, "./parse-mib", absPath)
	if err != nil {
		logger.Error(err)
		bomb("parse mib err")
	}
	if isTimeout {
		bomb("parse mib err: timeout")
	}

	var mibs []*models.Metric
	err = json.Unmarshal([]byte(out), &mibs)
	if err != nil {
		logger.Error(err, out)
		bomb("parse mib err")
	}
	os.RemoveAll(MIBS)

	for _, m := range mibs {
		newMib := models.NewMib(module, m)
		oldMib, err := models.MibGet("module=? and oid=?", module, m.Oid)
		if err != nil {
			logger.Warning("get mib err:", err)
			continue
		}
		if oldMib == nil {
			err := newMib.Save()
			if err != nil {
				logger.Warning("save mib err:", err)
			}
		}
	}
	renderMessage(c, err)
}

func mibModuleGet(c *gin.Context) {
	mibs, err := models.MibGetsGroupBy("module", "")
	dangerous(err)

	var modules []string
	for _, m := range mibs {
		if m.Module == "" {
			continue
		}
		modules = append(modules, m.Module)
	}

	renderData(c, modules, err)
}

func mibMetricGet(c *gin.Context) {
	module := queryStr(c, "module")

	mibs, err := models.MibGetsGroupBy("metric", "module=?", module)
	dangerous(err)

	var metric []string
	for _, m := range mibs {
		metric = append(metric, m.Metric)
	}
	renderData(c, metric, err)
}

func mibGet(c *gin.Context) {
	module := queryStr(c, "module", "")
	metric := queryStr(c, "metric", "")

	mib, err := models.MibGet("module=? and metric=?", module, metric)
	renderData(c, mib, err)
}

func mibGets(c *gin.Context) {
	module := queryStr(c, "module", "")
	metric := queryStr(c, "metric", "")

	var param []interface{}
	var sql string
	sql = "1 = 1"
	if module != "" {
		sql += " and module=?"
		param = append(param, module)
	}
	if metric != "" {
		sql += " and metric=?"
		param = append(param, metric)
	}

	mibs, err := models.MibGets(sql, param...)
	renderData(c, mibs, err)
}

func mibGetsByQuery(c *gin.Context) {
	loginUsername(c)

	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")

	total, err := models.MibTotal(query)
	dangerous(err)

	list, err := models.MibGetsByQuery(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type MibsDelRev struct {
	Ids []int64 `json:"ids"`
}

func mibDel(c *gin.Context) {
	username := loginUsername(c)
	can, err := models.UsernameCandoGlobalOp(username, "nems_network_ops")
	dangerous(err)
	if !can {
		bomb("no privilege")
	}

	var recv MibsDelRev
	dangerous(c.ShouldBind(&recv))
	for i := 0; i < len(recv.Ids); i++ {
		err = models.MibDel(recv.Ids[i])
		dangerous(err)
	}

	renderMessage(c, err)
}
