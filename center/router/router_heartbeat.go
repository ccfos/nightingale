package router

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io/ioutil"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/center/metas"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pushgw/idents"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

type HeartbeatHookFunc func(ident string) map[string]interface{}

func (rt *Router) heartbeat(c *gin.Context) {
	req, err := HandleHeartbeat(c, rt.Ctx, rt.Alert.Heartbeat.EngineName, rt.MetaSet, rt.IdentSet, rt.TargetCache)
	ginx.Dangerous(err)

	m := rt.HeartbeatHook(req.Hostname)
	ginx.NewRender(c).Data(m, err)
}

func HandleHeartbeat(c *gin.Context, ctx *ctx.Context, engineName string, metaSet *metas.Set, identSet *idents.Set, targetCache *memsto.TargetCacheType) (models.HostMeta, error) {
	var bs []byte
	var err error
	var r *gzip.Reader
	var req models.HostMeta
	if c.GetHeader("Content-Encoding") == "gzip" {
		r, err = gzip.NewReader(c.Request.Body)
		if err != nil {
			c.String(400, err.Error())
			return req, err
		}
		defer r.Close()
		bs, err = ioutil.ReadAll(r)
		ginx.Dangerous(err)
	} else {
		defer c.Request.Body.Close()
		bs, err = ioutil.ReadAll(c.Request.Body)
		if err != nil {
			return req, err
		}
	}

	err = json.Unmarshal(bs, &req)
	if err != nil {
		return req, err
	}

	if req.Hostname == "" {
		return req, errors.New("hostname is required")
	}

	// maybe from pushgw
	if req.Offset == 0 {
		req.Offset = (time.Now().UnixMilli() - req.UnixTime)
	}

	if req.RemoteAddr == "" {
		req.RemoteAddr = c.ClientIP()
	}

	if req.EngineName == "" {
		req.EngineName = engineName
	}

	metaSet.Set(req.Hostname, req)
	var items = make(map[string]struct{})
	items[req.Hostname] = struct{}{}
	identSet.MSet(items)

	if target, has := targetCache.Get(req.Hostname); has && target != nil {
		gid := ginx.QueryInt64(c, "gid", 0)
		hostIp := strings.TrimSpace(req.HostIp)

		field := make(map[string]interface{})
		if gid != 0 && gid != target.GroupId {
			field["group_id"] = gid
		}

		if hostIp != "" && hostIp != target.HostIp {
			field["host_ip"] = hostIp
		}

		tagsMap := target.GetTagsMap()
		tagNeedUpdate := false
		for k, v := range req.GlobalLabels {
			if v == "" {
				continue
			}

			if tagv, ok := tagsMap[k]; !ok || tagv != v {
				tagNeedUpdate = true
				tagsMap[k] = v
			}
		}

		if tagNeedUpdate {
			lst := []string{}
			for k, v := range tagsMap {
				lst = append(lst, k+"="+v)
			}
			labels := strings.Join(lst, " ") + " "
			field["tags"] = labels
		}

		if req.EngineName != "" && req.EngineName != target.EngineName {
			field["engine_name"] = req.EngineName
		}

		if req.AgentVersion != "" && req.AgentVersion != target.AgentVersion {
			field["agent_version"] = req.AgentVersion
		}

		if req.OS != "" && req.OS != target.OS {
			field["os"] = req.OS
		}

		if len(field) > 0 {
			err := target.UpdateFieldsMap(ctx, field)
			if err != nil {
				logger.Errorf("update target fields failed, err: %v", err)
			}
		}
		logger.Debugf("heartbeat field:%+v target: %v", field, *target)
	}

	return req, nil
}
