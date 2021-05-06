package http

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/config"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
)

type NidsRecv struct {
	Nids []string `json:"nids"`
}

type MetricList struct {
	Metrics []string `json:"metrics"`
}

func getMetrics(c *gin.Context) {
	recv := NidsRecv{}
	errors.Dangerous(c.ShouldBindJSON(&recv))

	var err error
	recv.Nids, err = GetChileNids(recv.Nids)
	errors.Dangerous(err)
	resp, err := Metrics(recv)

	renderData(c, resp, err)
}

func getTagkvs(c *gin.Context) {
	recv := NidMetricRecv{}
	errors.Dangerous(c.ShouldBindJSON(&recv))

	var err error
	recv.Nids, err = GetChileNids(recv.Nids)
	errors.Dangerous(err)
	resp, err := Tagkv(recv)

	renderData(c, resp, err)
}

type MetricsResp struct {
	Data MetricList `json:"dat"`
	Err  string     `json:"err"`
}

func Metrics(request NidsRecv) (MetricList, error) {
	addrs := GetIndexes()
	if len(addrs) == 0 {
		return MetricList{}, fmt.Errorf("empty index addr")
	}

	var result MetricsResp
	perm := rand.Perm(len(addrs))
	var err error
	for i := range perm {
		url := fmt.Sprintf("http://%s%s", addrs[perm[i]], "/api/index/metrics")
		err = httplib.Post(url).JSONBodyQuiet(request).SetTimeout(time.Duration(5000) * time.Millisecond).ToJSON(&result)
		if err == nil {
			break
		}
		logger.Warningf("index xclude failed, error:%v, req:%+v", err, request)
	}

	if err != nil {
		logger.Errorf("index xclude failed, error:%v, req:%+v", err, request)
		return MetricList{}, nil
	}

	if result.Err != "" {
		return MetricList{}, fmt.Errorf(result.Err)
	}
	return result.Data, nil
}

type NidMetricRecv struct {
	Nids    []string `json:"nids"`
	Metrics []string `json:"metrics"`
}

type IndexTagkvResp struct {
	Nids   []string           `json:"nids"`
	Metric string             `json:"metric"`
	Tagkv  []*dataobj.TagPair `json:"tagkv"`
}

type TagkvsResp struct {
	Data []IndexTagkvResp `json:"dat"`
	Err  string           `json:"err"`
}

func Tagkv(request NidMetricRecv) ([]IndexTagkvResp, error) {
	addrs := GetIndexes()
	if len(addrs) == 0 {
		return nil, fmt.Errorf("empty index addr")
	}

	var result TagkvsResp
	perm := rand.Perm(len(addrs))
	var err error
	for i := range perm {
		url := fmt.Sprintf("http://%s%s", addrs[perm[i]], "/api/index/tagkv")
		err = httplib.Post(url).JSONBodyQuiet(request).SetTimeout(time.Duration(5000) * time.Millisecond).ToJSON(&result)
		if err == nil {
			break
		}
		logger.Warningf("index xclude failed, error:%v, req:%+v", err, request)
	}

	if err != nil {
		return nil, fmt.Errorf("index xclude failed, error:%v, req:%+v", err, request)
	}

	if result.Err != "" {
		return nil, fmt.Errorf(result.Err)
	}
	return result.Data, nil
}

func GetIndexes() []string {
	var indexInstances []string
	instances, err := models.GetAllInstances(config.Config.Monapi.IndexMod, 1)
	if err != nil {
		return indexInstances
	}
	for _, instance := range instances {
		indexInstance := instance.Identity + ":" + instance.HTTPPort
		indexInstances = append(indexInstances, indexInstance)
	}
	return indexInstances
}

func GetChileNids(nids []string) ([]string, error) {
	childNids := []string{}
	ids := []int64{}
	filter := make(map[string]struct{})

	for _, nid := range nids {
		nidInt, err := strconv.ParseInt(nid, 10, 64)
		if err != nil {
			return childNids, err
		}
		ids = append(ids, nidInt)
		filter[nid] = struct{}{}
	}

	//
	var cnids []string
	nodes, err := models.NodeByIds(ids)
	if err != nil {
		return childNids, err
	}
	for _, node := range nodes {
		tmpNodes, err := node.RelatedNodes()
		if err != nil {
			return childNids, err
		}
		for _, n := range tmpNodes {
			cnids = append(cnids, strconv.FormatInt(n.Id, 10))
		}
	}

	for _, id := range cnids {
		filter[id] = struct{}{}
	}

	for nid, _ := range filter {
		childNids = append(childNids, nid)
	}
	return childNids, nil
}
