package cron

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/logger"
)

func SyncSnmpCollects() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL) * time.Second)

	syncSnmpCollects()
	logger.Info("[cron] sync snmp collects start...")
	for {
		<-t1.C
		syncSnmpCollects()
	}
}

func syncSnmpCollects() {
	snmpConfigs, err := models.GetSnmpCollects(0)
	if err != nil {
		logger.Warningf("get snmp collects err:%v", err)
		return
	}

	var snmpCollects []*dataobj.IPAndSnmp
	configsMap := make(map[string][]*dataobj.IPAndSnmp)
	for _, snmp := range snmpConfigs {
		snmp.Decode()
		hosts, err := HostUnderNode(snmp.Nid)
		if err != nil {
			logger.Warningf("get hosts err:%v %+v", err, snmp)
			continue
		}

		hws := models.GetHardwareInfoBy(hosts)

		for _, hw := range hws {
			if hw.Region == "" {
				continue
			}
			indexes := []*dataobj.Index{}
			lookups := []*dataobj.Lookup{}
			for _, snmpIdx := range snmp.Indexes {
				if snmpIdx == nil {
					continue
				}
				index := &dataobj.Index{
					Labelname: snmpIdx.TagKey,
					Type:      snmpIdx.Type,
				}
				indexes = append(indexes, index)

				for _, lookup := range snmpIdx.Lookups {
					if lookup == nil {
						continue
					}
					tmpLookup := &dataobj.Lookup{
						Labels:    []string{snmpIdx.TagKey},
						Labelname: lookup.Labelname,
						Oid:       lookup.Oid,
						Type:      lookup.Type,
					}
					lookups = append(lookups, tmpLookup)
				}
			}

			var enumValues map[int]string
			if snmp.OidType != 1 {
				mib, err := models.MibGet("module=? and metric=?", snmp.Module, snmp.Metric)
				if err != nil {
					logger.Warningf("get mib err:%v %+v", err, snmp)
					continue
				}

				if mib.Metric != "" {
					err = json.Unmarshal([]byte(mib.EnumValues), &enumValues)
					if err != nil {
						logger.Warningf("unmarshal enumValues err:%v %+v", err, mib)
					}
				}
			}

			metric := dataobj.Metric{
				Name:       snmp.Metric,
				Oid:        snmp.Oid,
				Type:       snmp.MetricType,
				Help:       snmp.Comment,
				Indexes:    indexes,
				EnumValues: enumValues,
				Lookups:    lookups,
			}

			if snmp.OidType == 1 {
				if m, exists := cache.ModuleMetric.Get(dataobj.COMMON_MODULE, snmp.Metric); exists {
					metric.Lookups = m.Lookups
					metric.EnumValues = m.EnumValues
					metric.Oid = m.Oid
					metric.Indexes = m.Indexes
					metric.Type = m.Type
				}
			}

			snmpCollect := &dataobj.IPAndSnmp{
				IP:          hw.IP,
				Version:     hw.SnmpVersion,
				Auth:        hw.Auth,
				Region:      hw.Region,
				Module:      snmp.Module,
				Step:        snmp.Step,
				Timeout:     snmp.Timeout,
				Port:        snmp.Port,
				Metric:      metric,
				LastUpdated: snmp.LastUpdated,
			}
			snmpCollects = append(snmpCollects, snmpCollect)
		}
	}

	for _, collect := range snmpCollects {
		if _, exists := cache.SnmpDetectorHashRing[collect.Region]; !exists {
			logger.Warningf("get node err, hash ring do noe exists %+v", collect)
			continue
		}
		pk := fmt.Sprintf("%s-%s-%s", collect.IP, collect.Module, collect.Metric.Oid)
		node, err := cache.SnmpDetectorHashRing[collect.Region].GetNode(pk)
		if err != nil {
			logger.Warningf("get node err:%v %v", err, collect)
			continue
		}

		key := collect.Region + "-" + node
		if _, exists := configsMap[key]; exists {
			configsMap[key] = append(configsMap[key], collect)
		} else {
			configsMap[key] = []*dataobj.IPAndSnmp{collect}
		}
	}

	cache.SnmpCollectCache.SetAll(configsMap)

}
