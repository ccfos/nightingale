package cron

import (
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/monapi/mcache"
	"github.com/didi/nightingale/src/toolkits/stats"
)

// SyncMaskconfLoop 周期性同步屏蔽策略，频率是9秒一次，一般采集周期不会低于10秒
func SyncMaskconfLoop() {
	duration := time.Second * time.Duration(9)
	for {
		time.Sleep(duration)
		logger.Debug("sync maskconf begin")
		err := SyncMaskconf()
		if err != nil {
			stats.Counter.Set("maskconf.sync.err", 1)
			logger.Error("sync maskconf fail: ", err)
		} else {
			logger.Debug("sync maskconf succ")
		}
	}
}

func SyncMaskconf() error {
	err := model.CleanExpireMask(time.Now().Unix())
	if err != nil {
		return fmt.Errorf("clean expire mask fail: %v", err)
	}

	mcs, err := model.MaskconfGetAll()
	if err != nil {
		return fmt.Errorf("get maskconf fail: %v", err)
	}

	// key: metric#endpoint
	// value: tags
	maskMap := make(map[string][]string)
	for i := 0; i < len(mcs); i++ {
		err := mcs[i].FillEndpoints()
		if err != nil {
			return fmt.Errorf("%v fill endpoints fail: %v", mcs[i], err)
		}

		for j := 0; j < len(mcs[i].Endpoints); j++ {
			key := mcs[i].Metric + "#" + mcs[i].Endpoints[j]
			maskMap[key] = append(maskMap[key], mcs[i].Tags)
		}
	}

	mcache.MaskCache.SetAll(maskMap)

	return nil
}

func IsMaskEvent(event *model.Event) bool {
	detail, err := event.GetEventDetail()
	if err != nil {
		logger.Errorf("get event detail failed, err: %v", err)
		return false
	}

	for i := 0; i < len(detail); i++ {
		eventMetric := detail[i].Metric
		var eventTagsList []string

		for k, v := range detail[i].Tags {
			eventTagsList = append(eventTagsList, fmt.Sprintf("%s=%s", strings.TrimSpace(k), strings.TrimSpace(v)))
		}
		key := eventMetric + "#" + event.Endpoint
		endpointKey := "#" + event.Endpoint
		maskTagsList, exists := mcache.MaskCache.GetByKey(endpointKey)
		if !exists {
			maskTagsList, exists = mcache.MaskCache.GetByKey(key)
			if !exists {
				continue
			}
		}

		for i := 0; i < len(maskTagsList); i++ {
			tagsList := strings.Split(maskTagsList[i], ",")
			if inList("", tagsList) {
				return true
			}

			if listContains(tagsList, eventTagsList) {
				return true
			}
		}
	}

	return false
}

// 用来判断blist是否包含slist
func listContains(slist, blist []string) bool {
	for i := 0; i < len(slist); i++ {
		if !inList(slist[i], blist) {
			return false
		}
	}

	return true
}

func inList(v string, lst []string) bool {
	for i := 0; i < len(lst); i++ {
		if lst[i] == v {
			return true
		}
	}

	return false
}
