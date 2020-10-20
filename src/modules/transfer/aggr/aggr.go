package aggr

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/cache"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/spaolacci/murmur3"
	"github.com/toolkits/pkg/logger"
)

type AggrSection struct {
	Enabled           bool     `yaml:"enabled"`
	ApiPath           string   `yaml:"apiPath"`
	ApiTimeout        int      `yaml:"apiTimeout"`
	UpdateInterval    int      `yaml:"updateInterval"`
	KafkaAddrs        []string `yaml:"kafkaAddrs"`
	KafkaAggrInTopic  string   `yaml:"kafkaAggrInTopic"`
	KafkaAggrOutTopic string   `yaml:"kafkaAggrOutTopic"`
}

var AggrConfig AggrSection

func Init(aggr AggrSection) {
	AggrConfig = aggr
	if !AggrConfig.Enabled {
		return
	}

	InitKakfa(AggrConfig)
}

// SendToCentralAggr
// 1. 判断一组points是否应该发往中心进行聚合
func SendToAggr(items []*dataobj.MetricValue) error {

	//配置了计算策略的指标过滤
	var aggrList dataobj.AggrList

	for _, item := range items {
		var key string
		if item.Nid != "" {
			key = item.Nid
		} else if item.Endpoint != "" {
			key = item.Endpoint
		}

		validStrategys := cache.AggrCalcMap.GetByKey(str.MD5(key, item.Metric, ""))
		var stras []*dataobj.RawMetricAggrCalc
		for _, stra := range validStrategys {
			if !tagMatch(stra.TagFilters, item.TagsMap) {
				continue
			}
			stras = append(stras, stra)
		}
		if len(stras) == 0 {
			continue
		}

		aggrPoint, err := transPoints(item, stras)
		if err != nil {
			logger.Warning(err)
			continue
		}

		aggrList.Data = append(aggrList.Data, aggrPoint)
	}

	if len(aggrList.Data) == 0 {
		return nil
	}

	pBytes, err := json.Marshal(aggrList)
	if err != nil {
		logger.Errorf("marshal points err:", err)
		return err
	}

	KafkaProducer.producePoints(pBytes)

	psize := len(aggrList.Data)
	logger.Debug(psize)

	return nil
}

// transPoints
// 转换原始points到新的points结构
func transPoints(item *dataobj.MetricValue, strategys []*dataobj.RawMetricAggrCalc) (*dataobj.CentralAggrV2Point, error) {
	result := &dataobj.CentralAggrV2Point{}
	// 判断时间戳是否在可接受的范围内
	// 认为正负7天之外的数据是不能接受的
	nowTs := time.Now().Unix()
	if item.Timestamp > (nowTs+7*86400) || item.Timestamp < (nowTs-7*86400) {
		err := fmt.Errorf("invalid ts point found, point: %+v\n", item)
		return result, err
	}

	if len(strategys) == 0 {
		err := fmt.Errorf("strategy is null")
		return result, err
	}

	result.Timestamp = item.Timestamp
	result.Value = item.Value
	result.Strategys = make([]*dataobj.AggrCalcStra, 0)

	// 根据counter生成hash，用于去重
	var hash string
	if item.Nid != "" {
		hash = item.Nid + item.Metric
	} else {
		hash = item.Endpoint + item.Metric
	}

	hashKeys := make([]string, 0)
	for k := range item.TagsMap {
		hashKeys = append(hashKeys, k)
	}
	sort.Strings(hashKeys)
	for _, k := range hashKeys {
		hash += k + item.TagsMap[k]
	}

	result.Hash = murmur3.Sum64([]byte(hash)) / 2

	for _, rule := range strategys {
		ruleValid := true

		// 检查 group by 的 key, 如果key不存在，规则不合法
		groupKey := make([]string, 0)
		for _, tagk := range rule.GroupBy {
			tagv, exist := item.TagsMap[tagk]
			if !exist {
				logger.Errorf("drop item %+v: no tagk= %s found. sid = %d\n", item, tagk, rule.Sid)
				ruleValid = false
				break // groupby tagk1，但这个点没有tagk1，跳过这条计算规则
			}
			groupKey = append(groupKey, tagk+"="+tagv)
		}
		if !ruleValid {
			continue
		}

		// 设置step
		step := rule.NewStep
		if rule.NewStep == 0 {
			step = int(item.Step) // 如果没有指定step，则默认和point的step相同
		}

		lateness := step * 3

		result.Strategys = append(result.Strategys, &dataobj.AggrCalcStra{
			SID:            rule.Sid,
			NID:            strconv.FormatInt(rule.Nid, 10),
			ResultStep:     step,
			RawStep:        int(item.Step),
			GroupKey:       strings.Join(groupKey, "||"),
			GlobalOperator: rule.GlobalOperator,
			InnerOperator:  rule.InnerOperator,
			VarID:          rule.VarID,
			VarNum:         rule.VarNum,
			RPN:            rule.RPN,
			Lateness:       lateness,
		})
	}

	if len(result.Strategys) == 0 {
		return result, fmt.Errorf("strategys is null")
	}
	return result, nil
}

func tagMatch(straTags []*dataobj.AggrTagsFilter, tag map[string]string) bool {
	for _, stag := range straTags {
		if _, exists := tag[stag.TagK]; !exists {
			return false
		}
		var match bool
		if stag.Opt == "=" { //当前策略 tagkey 对应的 tagv
			for _, v := range stag.TagV {
				if tag[stag.TagK] == v {
					match = true
					break
				}
			}
		} else {
			match = true
			for _, v := range stag.TagV {
				if tag[stag.TagK] == v {
					match = false
					return match
				}
			}
		}

		if !match {
			return false
		}
	}
	return true
}
