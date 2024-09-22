package router

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/ccfos/nightingale/v6/center/metas"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

// heartbeat Forward heartbeat request to the center.
func (rt *Router) heartbeat(c *gin.Context) {
	gid := ginx.QueryStr(c, "gid", "")
	overwriteGids := ginx.QueryBool(c, "overwrite_gids", false)
	req, err := HandleHeartbeat(c, rt.Aconf.Heartbeat.EngineName, rt.MetaSet)
	if err != nil {
		logger.Warningf("req:%v heartbeat failed to handle heartbeat err:%v", req, err)
		ginx.Dangerous(err)
	}
	api := "/v1/n9e/center/heartbeat"
	if rt.HeartbeartApi != "" {
		api = rt.HeartbeartApi
	}

	ret, err := poster.PostByUrlsWithResp[map[string]interface{}](rt.Ctx, fmt.Sprintf("%s?gid=%s&overwrite_gids=%t", api, gid, overwriteGids), req)
	ginx.NewRender(c).Data(ret, err)
}

func HandleHeartbeat(c *gin.Context, engineName string, metaSet *metas.Set) (models.HostMeta, error) {
	var bs []byte
	var err error
	var r *gzip.Reader
	var req models.HostMeta
	if c.GetHeader("Content-Encoding") == "gzip" {
		r, err = gzip.NewReader(c.Request.Body)
		if err != nil {
			return req, err
		}

		defer r.Close()
		bs, err = io.ReadAll(r)
		if err != nil {
			return req, err
		}
	} else {
		defer c.Request.Body.Close()
		bs, err = io.ReadAll(c.Request.Body)
		if err != nil {
			return req, err
		}
	}

	err = json.Unmarshal(bs, &req)
	if err != nil {
		return req, err
	}

	if req.Hostname == "" {
		ginx.Dangerous("hostname is required", 400)
	}

	req.Offset = (time.Now().UnixMilli() - req.UnixTime)
	req.RemoteAddr = c.ClientIP()
	req.EngineName = engineName
	metaSet.Set(req.Hostname, req)

	return req, nil
}
