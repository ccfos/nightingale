package router

import (
	"compress/gzip"
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) heartbeat(c *gin.Context) {
	var bs []byte
	var err error
	var r *gzip.Reader
	var req models.HostMeta
	if c.GetHeader("Content-Encoding") == "gzip" {
		r, err = gzip.NewReader(c.Request.Body)
		if err != nil {
			c.String(400, err.Error())
			return
		}
		defer r.Close()
		bs, err = ioutil.ReadAll(r)
		ginx.Dangerous(err)
	} else {
		defer c.Request.Body.Close()
		bs, err = ioutil.ReadAll(c.Request.Body)
		ginx.Dangerous(err)
	}

	err = json.Unmarshal(bs, &req)
	ginx.Dangerous(err)

	req.Offset = (time.Now().UnixMilli() - req.UnixTime)
	req.RemoteAddr = c.Request.RemoteAddr

	gid := ginx.QueryInt64(c, "gid", 0)
	req.Bgid = gid
	rt.MetaSet.Set(req.Hostname, req)

	needBgidup := false
	if gid != 0 {
		target, has := rt.TargetCache.Get(req.Hostname)
		if has && target.Bgid != gid {
			needBgidup = true
		}
	}

	if needBgidup {
		err = models.TargetUpdateBgid(rt.Ctx, []string{req.Hostname}, gid, false)
	}

	ginx.NewRender(c).Message(err)
}
