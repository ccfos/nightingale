package router

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// heartbeat Forward heartbeat request to the center.
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
		bs, err = io.ReadAll(r)
		ginx.Dangerous(err)
	} else {
		defer c.Request.Body.Close()
		bs, err = io.ReadAll(c.Request.Body)
		ginx.Dangerous(err)
	}

	err = json.Unmarshal(bs, &req)
	ginx.Dangerous(err)

	if req.Hostname == "" {
		ginx.Dangerous("hostname is required", 400)
	}

	req.Offset = (time.Now().UnixMilli() - req.UnixTime)
	req.RemoteAddr = c.ClientIP()
	gid := ginx.QueryStr(c, "gid", "")

	req.EngineName = rt.Aconf.Heartbeat.EngineName

	rt.MetaSet.Set(req.Hostname, req)

	ginx.NewRender(c).Message(poster.PostByUrls(rt.Ctx, "/v1/n9e/heartbeat?gid="+gid, req))
}
