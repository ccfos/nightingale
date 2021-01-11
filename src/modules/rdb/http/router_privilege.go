package http

import (
	"fmt"

	"github.com/didi/nightingale/src/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

func privilegeGets(c *gin.Context) {
	typ := queryStr(c, "typ", "global")
	ret, err := models.PrivilegeGets("typ=?", typ)
	dangerous(err)

	renderData(c, ret, nil)
}

func privilegePost(c *gin.Context) {
	var fs []*models.Privilege
	bind(c, &fs)

	me := loginUsername(c)
	for _, f := range fs {
		f.LastUpdater = me
	}

	err := models.PrivilegeAdds(fs)
	dangerous(err)

	renderMessage(c, nil)
}

func privilegePut(c *gin.Context) {
	var f models.Privilege
	bind(c, &f)

	me := loginUsername(c)
	p, err := models.PrivilegeGet("id=?", f.Id)
	dangerous(err)

	if p == nil {
		dangerous(fmt.Errorf("privilege is nil"))
	}

	p.Pid = f.Pid
	p.Typ = f.Typ
	p.Cn = f.Cn
	p.En = f.En
	p.Weight = f.Weight
	p.Path = f.Path
	p.Leaf = f.Leaf
	p.LastUpdater = me
	err = p.Update("pid", "typ", "cn", "en", "weight", "path", "leaf", "last_updater")
	dangerous(err)

	renderMessage(c, nil)
}

func privilegeDel(c *gin.Context) {
	var fs []int64
	bind(c, &fs)

	me := loginUsername(c)
	err := models.PrivilegeDels(fs)
	dangerous(err)

	logger.Infof("[rdb] %s delete privilege %+v", me, fs)

	renderMessage(c, nil)
}

type PrivilegeWeight struct {
	Id     int64 `json:"id"`
	Weight int   `json:"weight"`
}

func privilegeWeights(c *gin.Context) {
	var fs []PrivilegeWeight
	bind(c, &fs)

	me := loginUsername(c)
	cnt := len(fs)
	for i := 0; i < cnt; i++ {
		privilege, err := models.PrivilegeGet("id=?", fs[i].Id)
		dangerous(err)

		if privilege == nil {
			dangerous(fmt.Errorf("privilege is nil"))
		}

		// 加权限点控制  参考canWriteChart
		// can, err := canWritePrivilege(chart.SubclassId, me)
		// dangerous(err)
		// if !can {
		// 	dangerous("forbidden")
		// }
	}

	for i := 0; i < cnt; i++ {
		privilege, err := models.PrivilegeGet("id=?", fs[i].Id)
		dangerous(err)
		privilege.Weight = fs[i].Weight
		errors.Dangerous(privilege.Update("weight"))
	}

	logger.Infof("[rdb] %s change privilege weight %+v", me, fs)

	renderMessage(c, nil)
}
