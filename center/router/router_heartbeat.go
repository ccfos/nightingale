package router

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io/ioutil"
	"sort"
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

		newTarget := models.Target{}
		targetNeedUpdate := false
		if gid != 0 && gid != target.GroupId {
			newTarget.GroupId = gid
			targetNeedUpdate = true
		}

		if hostIp != "" && hostIp != target.HostIp {
			newTarget.HostIp = hostIp
			targetNeedUpdate = true
		}

		hostTagsMap := target.GetHostTagsMap()
		hostTagNeedUpdate := false
		if len(hostTagsMap) != len(req.GlobalLabels) {
			hostTagNeedUpdate = true
		} else {
			for k, v := range req.GlobalLabels {
				if v == "" {
					continue
				}

				if tagv, ok := hostTagsMap[k]; !ok || tagv != v {
					hostTagNeedUpdate = true
					break
				}
			}
		}

		if hostTagNeedUpdate {
			lst := []string{}
			for k, v := range req.GlobalLabels {
				lst = append(lst, k+"="+v)
			}
			sort.Strings(lst)
			newTarget.HostTags = lst
			targetNeedUpdate = true
		}

		userTagsMap := target.GetTagsMap()
		userTagNeedUpdate := false
		userTags := []string{}
		for k, v := range userTagsMap {
			if v == "" {
				continue
			}

			if _, ok := req.GlobalLabels[k]; !ok {
				userTags = append(userTags, k+"="+v)
			} else { // 该key在hostTags中已经存在
				userTagNeedUpdate = true
			}
		}

		if userTagNeedUpdate {
			newTarget.Tags = strings.Join(userTags, " ") + " "
			targetNeedUpdate = true
		}

		if req.EngineName != "" && req.EngineName != target.EngineName {
			newTarget.EngineName = req.EngineName
			targetNeedUpdate = true
		}

		if req.AgentVersion != "" && req.AgentVersion != target.AgentVersion {
			newTarget.AgentVersion = req.AgentVersion
			targetNeedUpdate = true
		}

		if req.OS != "" && req.OS != target.OS {
			newTarget.OS = req.OS
			targetNeedUpdate = true
		}

		if targetNeedUpdate {
			err := models.DB(ctx).Model(&target).Updates(newTarget).Error
			if err != nil {
				logger.Errorf("update target fields failed, err: %v", err)
			}
		}
		logger.Debugf("heartbeat field:%+v target: %v", newTarget, *target)
	}

	return req, nil
}
