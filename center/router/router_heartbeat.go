package router

import (
	"compress/gzip"
	"encoding/json"
	"io/ioutil"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
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
		gid := ginx.QueryInt64(c, "gid", 0)
		hostIp := strings.TrimSpace(req.HostIp)

		filed := make(map[string]interface{})
		if gid != 0 && gid != target.GroupId {
			filed["group_id"] = gid
		}

		if hostIp != "" && hostIp != target.HostIp {
			filed["host_ip"] = hostIp
		}

		if len(req.GlobalLabels) > 0 {
			lst := []string{}
			for k, v := range req.GlobalLabels {
				lst = append(lst, k+"="+v)
			}
			sort.Strings(lst)
			labels := strings.Join(lst, " ")
			if target.Tags != labels {
				filed["tags"] = labels
			}
		}

		if len(filed) > 0 {
			err := target.UpdateFieldsMap(rt.Ctx, filed)
			if err != nil {
				logger.Errorf("update target fields failed, err: %v", err)
			}
		}
		logger.Debugf("heartbeat field:%+v target: %v", filed, *target)
	}

	ginx.NewRender(c).Message(err)
}
