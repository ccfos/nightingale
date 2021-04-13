package alarm

import (
	"fmt"
	"strings"
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/logger"
)

func SyncMaskconfLoop() {
	for {
		SyncMaskconf()
		time.Sleep(time.Second * time.Duration(9))
	}
}

func SyncMaskconf() error {
	err := models.CleanExpireMask(time.Now().Unix())
	if err != nil {
		logger.Errorf("clean expire mask fail, err: %v", err)
		return err
	}

	mcs, err := models.MaskconfGetAll()
	if err != nil {
		logger.Errorf("get maskconf fail, err: %v", err)
		return err
	}

	// key: metric#endpoint
	// value: tags
	maskMap := make(map[string][]string)
	for i := 0; i < len(mcs); i++ {
		if mcs[i].Category == 1 {
			err := mcs[i].FillEndpoints()
			if err != nil {
				return fmt.Errorf("%v fill endpoints fail: %v", mcs[i], err)
			}

			for j := 0; j < len(mcs[i].Endpoints); j++ {
				key := mcs[i].Metric + "#" + mcs[i].Endpoints[j]
				maskMap[key] = append(maskMap[key], mcs[i].Tags)
			}
		} else {
			err := mcs[i].FillNids()
			if err != nil {
				return fmt.Errorf("%v fill endpoints fail: %v", mcs[i], err)
			}

			for nid, _ := range mcs[i].CurNidPaths {
				key := mcs[i].Metric + "#" + nid
				maskMap[key] = append(maskMap[key], mcs[i].Tags)
			}
		}
	}

	cache.MaskCache.SetAll(maskMap)

	return nil
}

func IsMaskEvent(event *models.Event) bool {
	detail, err := event.GetEventDetail()
	if err != nil {
		logger.Errorf("get event detail:%v failed, err: %v", event.Detail, err)
		return false
	}

	for i := 0; i < len(detail); i++ {
		eventMetric := detail[i].Metric
		eventTagsList := []string{}

		for k, v := range detail[i].Tags {
			eventTagsList = append(eventTagsList, fmt.Sprintf("%s=%s", strings.TrimSpace(k), strings.TrimSpace(v)))
		}

		var maskTagsList []string
		var exists bool
		if event.Category == 1 {
			maskTagsList, exists = cache.MaskCache.GetByKey("#" + event.Endpoint)
			if !exists {
				maskTagsList, exists = cache.MaskCache.GetByKey(eventMetric + "#" + event.Endpoint)
				if !exists {
					continue
				}
			}
		} else {
			maskTagsList, exists = cache.MaskCache.GetByKey(eventMetric + "#" + event.CurNid)
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
