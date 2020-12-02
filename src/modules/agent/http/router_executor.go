package http

import (
	"fmt"
	"log"
	"path"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"

	"github.com/didi/nightingale/src/modules/agent/config"
	"github.com/didi/nightingale/src/modules/agent/timer"
)

func output(idstr string, typ string) (string, error) {
	fp := path.Join(config.Config.Job.MetaDir, idstr, typ)
	if file.IsExist(fp) {
		return file.ToString(fp)
	}

	id, err := strconv.ParseInt(idstr, 10, 64)
	if err != nil {
		return "", err
	}

	t, has := timer.Locals.GetTask(id)
	if !has {
		return "", nil
	}

	if typ == "stdout" {
		return string(t.GetStdout()), nil
	}

	return string(t.GetStderr()), nil
}

func stdoutTxt(c *gin.Context) {
	content, err := output(urlParamStr(c, "id"), "stdout")
	if err != nil {
		log.Println("[E]", err)
		c.AbortWithError(500, fmt.Errorf("read stdout fail: %v", err))
		return
	}

	c.String(200, content)
}

func stdoutJSON(c *gin.Context) {
	content, err := output(urlParamStr(c, "id"), "stdout")
	renderData(c, content, err)
}

func stderrTxt(c *gin.Context) {
	content, err := output(urlParamStr(c, "id"), "stderr")
	if err != nil {
		log.Println("[E]", err)
		c.AbortWithError(500, fmt.Errorf("read stderr fail: %v", err))
		return
	}

	c.String(200, content)
}

func stderrJSON(c *gin.Context) {
	content, err := output(urlParamStr(c, "id"), "stderr")
	renderData(c, content, err)
}
