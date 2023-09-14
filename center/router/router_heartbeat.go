package router

import (
	"compress/gzip"
	"encoding/json"
	"io/ioutil"
	"strings"
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

	// maybe from pushgw
	if req.Offset == 0 {
		req.Offset = (time.Now().UnixMilli() - req.UnixTime)
	}

	if req.RemoteAddr == "" {
		req.RemoteAddr = c.ClientIP()
	}

	rt.MetaSet.Set(req.Hostname, req)
	var items = make(map[string]struct{})
	items[req.Hostname] = struct{}{}
	rt.IdentSet.MSet(items)

	if target, has := rt.TargetCache.Get(req.Hostname); has && target != nil {
		var defGid int64 = -1
		gid := ginx.QueryInt64(c, "gid", defGid)
		hostIpStr := strings.TrimSpace(req.HostIp)
		if gid == defGid { //set gid value from cache
			gid = target.GroupId
		}
		if gid != target.GroupId || hostIpStr != target.HostIp { // if either gid or host_ip has a new value
			err = models.TargetUpdateHostIpAndBgid(rt.Ctx, req.Hostname, hostIpStr, gid)
		}
	}

	ginx.NewRender(c).Message(err)
}
