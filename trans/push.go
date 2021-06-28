package trans

import (
	"bytes"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/backend"
	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"
	"github.com/didi/nightingale/v5/naming"
	"github.com/didi/nightingale/v5/vos"
)

func Push(points []*vos.MetricPoint) error {
	if points == nil {
		return fmt.Errorf("param(points) is nil")
	}

	count := len(points)
	if count == 0 {
		return fmt.Errorf("param(points) is empty")
	}

	var reterr error

	// 把ident->alias做成map，放内存里，后续要周期性与DB中的数据对比，更新resource表
	aliasMapper := make(map[string]interface{})

	now := time.Now().Unix()
	validPoints := make([]*vos.MetricPoint, 0, count)
	for i := 0; i < count; i++ {
		logger.Debugf("recv %+v", points[i])
		// 如果tags中发现有__ident__和__alias__就提到外层，这个逻辑是为了应对snmp之类的场景
		if val, has := points[i].TagsMap["__ident__"]; has {
			points[i].Ident = val
			delete(points[i].TagsMap, "__ident__")
		}

		if val, has := points[i].TagsMap["__alias__"]; has {
			points[i].Alias = val
			delete(points[i].TagsMap, "__alias__")
		}

		if err := points[i].Tidy(now); err != nil {
			// 如果有部分point校验失败，没关系，把error返回即可，正常的可以继续往下走
			logger.Warningf("point %+v is invalid, err:%v ", points[i], err)
			reterr = err
		} else {
			if points[i].Ident != "" {
				// 把当前时间也带上，处理的时候只处理最近的数据，避免alias发生变化且数据分散在多个server造成的alias不一致的问题
				aliasMapper[points[i].Ident] = &models.AliasTime{Alias: points[i].Alias, Time: now}
			}
			// 将resource的tag追加到曲线的tag中，根据tagsmap生成tagslst，排序，生成primarykey
			enrich(points[i])
			validPoints = append(validPoints, points[i])
		}
	}

	models.AliasMapper.MSet(aliasMapper)

	// 路由数据，做转发的逻辑可以做成异步，这个过程如果有错，都是系统内部错误，不需要暴露给client侧
	go DispatchPoints(validPoints)

	return reterr
}

func DispatchPoints(points []*vos.MetricPoint) {
	// send to push endpoints
	pushEndpoints, err := backend.GetPushEndpoints()
	if err != nil {
		logger.Errorf("could not find pushendpoint:%v", err)
	} else {
		for _, pushendpoint := range pushEndpoints {
			go pushendpoint.Push2Queue(points)
		}
	}

	// send to judge queue
	for i := range points {
		node, err := naming.HashRing.GetNode(points[i].PK)
		if err != nil {
			logger.Errorf("could not find node:%v", err)
			continue
		}

		q, exists := queues.Get(node)
		if !exists {
			logger.Errorf("could not find queue by %s", node)
			continue
		}

		q.PushFront(points[i])
	}
}

var bufferPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

func enrich(point *vos.MetricPoint) {
	// 把res的tags附到point上
	resAndTags, exists := cache.ResTags.Get(point.Ident)
	if exists {
		for k, v := range resAndTags.Tags {
			point.TagsMap[k] = v
		}
	}

	// 根据tagsmap生成tagslst，sort
	count := len(point.TagsMap)
	if count == 0 {
		point.TagsLst = []string{}
	} else {
		lst := make([]string, 0, count)
		for k, v := range point.TagsMap {
			lst = append(lst, k+"="+v)
		}
		sort.Strings(lst)
		point.TagsLst = lst
	}

	// ident metric tagslst 生成 pk
	ret := bufferPool.Get().(*bytes.Buffer)
	ret.Reset()
	defer bufferPool.Put(ret)

	ret.WriteString(point.Ident)
	ret.WriteString(point.Metric)

	for i := 0; i < len(point.TagsLst); i++ {
		ret.WriteString(point.TagsLst[i])
	}

	point.PK = str.MD5(ret.String())
}
