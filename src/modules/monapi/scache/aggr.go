package scache

import (
	"strconv"
	"time"

	"github.com/didi/nightingale/src/models"

	"github.com/toolkits/pkg/logger"
)

func SyncAggrCalcStras() {
	t1 := time.NewTicker(time.Duration(CHECK_INTERVAL) * time.Second)

	syncAggrCalcStras()
	logger.Info("[cron] sync stras start...")
	for {
		<-t1.C
		syncAggrCalcStras()
	}
}

func syncAggrCalcStras() {
	stras, err := models.AggrCalcsList("", 0)
	if err != nil {
		logger.Error("sync stras err:", err)
		return
	}

	for i := 0; i < len(stras); i++ {

		stras[i].VarNum = getVarNum(stras[i].RPN)
		if stras[i].Category == 2 {
			for j := 0; j < len(stras[i].RawMetrics); j++ {
				//只有非nodata的告警策略，才支持告警策略继承，否则nodata会有误报
				nids, err := models.GetRelatedNidsForMon(stras[i].RawMetrics[j].Nid, stras[i].RawMetrics[j].ExclNid)
				if err != nil {
					logger.Warningf("get LeafNids err:%v %v", err, stras[i].RawMetrics[j])
					continue
				}

				for _, nid := range nids {
					stras[i].RawMetrics[j].Nids = append(stras[i].RawMetrics[j].Nids, strconv.FormatInt(nid, 10))
				}

			}
		} else if stras[i].Category == 1 {
			for j := 0; j < len(stras[i].RawMetrics); j++ {
				//增加叶子节点nid
				leafNids, err := models.GetLeafNidsForMon(stras[i].RawMetrics[j].Nid, stras[i].RawMetrics[j].ExclNid)
				if err != nil {
					logger.Warningf("get LeafNids err:%v %v", err, stras[i])
					continue
				}

				var hosts []string
				for _, nid := range leafNids {
					hs, err := HostUnderNode(nid)
					if err != nil {
						logger.Warningf("get hosts err:%v %v", err, stras[i])
						continue
					}
					hosts = append(hosts, hs...)
				}

				hostFilter := make(map[string]struct{})
				for _, host := range hosts {
					if _, exists := hostFilter[host]; exists {
						continue
					}
					hostFilter[host] = struct{}{}
					stras[i].RawMetrics[j].Endpoints = append(stras[i].RawMetrics[j].Endpoints, host)
				}

			}

		}
	}

	AggrCalcStraCache.Set(stras)
}

func CleanAggrCalcStraLoop() {
	duration := time.Second * time.Duration(300)
	for {
		time.Sleep(duration)
		cleanAggrCalcStra()
	}
}

//定期清理没有找到nid的策略
func cleanAggrCalcStra() {
	list, err := models.AggrCalcsList("", 0)
	if err != nil {
		logger.Errorf("get stras fail: %v", err)
		return
	}

	for _, stra := range list {
		node, err := models.NodeGet("id=?", stra.Nid)
		if err != nil {
			logger.Warningf("get node failed, node id: %d, err: %v", stra.Nid, err)
			continue
		}

		if node == nil {
			logger.Infof("delete aggr_calc stra:%d", stra.Id)
			if err := models.AggrCalcDel(stra.Id); err != nil {
				logger.Warningf("delete stra: %d, err: %v", stra.Id, err)
			}
		}
	}
}

func getVarNum(RPN string) int {
	cnt := 0
	for _, c := range RPN {
		if c == '$' || c == '#' {
			cnt++
		}
	}
	return cnt
}
