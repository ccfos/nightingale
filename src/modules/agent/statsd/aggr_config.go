package statsd

/*
// raw configs
type MetricAgentConfig struct {
	Updated  int64                      `json:"updated"` // 配置生成的时间戳
	Version  string                     `json:"version"` // 配置版本
	Hostname string                     `json:"hostname"`
	Ip       string                     `json:"ip"`
	Aggr     map[string]*AggrConfigItem `json:"aggr"` // ns --> x
}
type AggrConfigItem struct {
	Ns          string                      `json:"ns"`
	Type        string                      `json:"type"`
	MetricTagks map[string]*AggrMetricTagks `json:"metric_tagks"`
}
type AggrMetricTagks struct {
	Metric string     `json:"metric"`
	Tagks  [][]string `json:"tagks"`
}

func (this MetricAgentConfig) UpdateLoop() {
	if sconfig.Config.Cfg.Disable {
		logger.Debugf("config update loop disabled")
		return
	}
	for {
		nc, err := this.getMetricAgentConfigFromRemote()
		if err != nil {
			logger.Debugf("get metric agent config error, [error: %s]", err.Error())
		} else if nc == nil {
			// 机器没有配置metrics本机聚合
		} else {
			lac, err1 := nc.transToLocalAggrConfig()
			if err1 != nil {
				logger.Debugf("trans to local aggr config error, [error: %s]", err1.Error())
			} else {
				localAggrConfig.Update(lac, nc.Version, nc.Updated)
				logger.Debugf("localAggrConfig updated at:%d", nc.Updated)
			}
		}
		time.Sleep(time.Duration(sconfig.Config.Cfg.UdpateIntervalMs) * time.Millisecond)
	}
}

func (this *MetricAgentConfig) transToLocalAggrConfig() (map[string]*NsAggrConfig, error) {
	if len(this.Aggr) == 0 && this.Updated == 0 && this.Version == "" {
		return nil, fmt.Errorf("bad aggr configs")
	}

	ret := make(map[string]*NsAggrConfig, 0)
	for _, v := range this.Aggr {
		if !(LocalAggrConfig{}.CheckType(v.Type)) {
			logger.Debugf("bad aggr config type, [type: %s]", v.Type)
			continue
		}

		// metric_tagks
		mtks := make(map[string][][]string, 0)
		for _, mtk := range v.MetricTagks {
			if mtk == nil || len(mtk.Metric) == 0 || len(mtk.Tagks) == 0 {
				continue
			}

			ttagks := make([][]string, 0)
			for i := 0; i < len(mtk.Tagks); i++ {
				mtksTagksMap := make(map[string]bool, 0)
				for _, tk := range mtk.Tagks[i] {
					mtksTagksMap[tk] = true
				}
				mktsTagsList := make([]string, 0)
				for k, _ := range mtksTagksMap {
					mktsTagsList = append(mktsTagsList, k)
				}
				sort.Strings(mktsTagsList)
				ttagks = append(ttagks, mktsTagsList)
			}
			if (Func{}).HasSameSortedArray(ttagks) {
				logger.Debugf("bad aggr config tagks, has same tagks: [ns: %s][metric: %s][tagks: %#v]",
					v.Ns, mtk.Metric, mtk.Tagks)
				logger.Debugf("drop aggr config of metric, [ns: %s][metric: %s]", v.Ns, mtk.Metric)
				continue
			}
			mtks[mtk.Metric] = ttagks
		}
		if attks, ok := mtks[Const_AllMetrics]; ok && len(attks) > 0 {
			for k, v := range mtks {
				if k == Const_AllMetrics {
					continue
				}
				mtks[k] = (Func{}).MergeSortedArrays(attks, v)
			}
		}

		// metric_tagks
		ret[v.Ns] = &NsAggrConfig{
			Ns:          v.Ns,
			Type:        v.Type,
			MetricTagks: mtks,
		}
	}
	return ret, nil
}

// local transfered configs
var (
	localAggrConfig = &LocalAggrConfig{NsConfig: map[string]*NsAggrConfig{}, Updated: 0, Version: "init"}
)

func (this LocalAggrConfig) GetLocalAggrConfig() *LocalAggrConfig {
	return localAggrConfig.Clone()
}

const (
	// Type: 三段式 ${指标}:${聚合维度}:${聚合与否}
	Const_AggrType_AllAnyNoaggr = "all:any:noaggr"
	Const_AggrType_SomeSomeAggr = "some:some:aggr"

	// 全部指标
	Const_AllMetrics = ".*"
)

var (
	// 禁止聚合-常亮
	Const_NoAggrConfig = &NsAggrConfig{Ns: ".*", Type: Const_AggrType_AllAnyNoaggr}
)

type LocalAggrConfig struct {
	sync.RWMutex
	NsConfig map[string]*NsAggrConfig `json:"ns_config"`
	Version  string                   `json:"version"`
	Updated  int64                    `json:"updated"`
}
type NsAggrConfig struct {
	Ns          string                `json:"ns"`
	Type        string                `json:"type"`
	MetricTagks map[string][][]string `json:"metric_tagks"`
}

func (this *LocalAggrConfig) GetByNs(ns string) (nsAggrConfig *NsAggrConfig, found bool) {
	// TODO: daijia产品线自己做了聚合,因此metrics不再聚合
	if strings.HasSuffix(ns, ".daijia.n9e.com") {
		nsAggrConfig = Const_NoAggrConfig
		found = true
		return
	}

	this.RLock()
	nsAggrConfig, found = this.NsConfig[ns]
	this.RUnlock()
	return
}

func (this *LocalAggrConfig) Update(nac map[string]*NsAggrConfig, version string, updated int64) {
	this.Lock()
	this.NsConfig = nac
	this.Version = version
	this.Updated = updated
	this.Unlock()
}

func (this *LocalAggrConfig) Clone() *LocalAggrConfig {
	ret := &LocalAggrConfig{}
	this.RLock()
	ret.Updated = this.Updated
	ret.NsConfig = this.NsConfig
	this.RUnlock()
	return ret
}

func (this LocalAggrConfig) CheckType(t string) bool {
	switch t {
	case Const_AggrType_AllAnyNoaggr, Const_AggrType_SomeSomeAggr:
		return true
	}
	return false
}
*/
