package timer

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"

	"github.com/toolkits/pkg/logger"
)

// 从数据库同步资源表的信息，组成res_ident->res_tags结构，
// 监控数据上报时，会根据ident找到资源标签，附到监控数据的标签里
func SyncResourceTags() {
	err := syncResourceTags()
	if err != nil {
		fmt.Println("timer: sync res tags fail:", err)
		exit(1)
	}

	go loopSyncResourceTags()
}

func loopSyncResourceTags() {
	randtime := rand.Intn(9000)
	fmt.Printf("timer: sync res tags: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	for {
		time.Sleep(time.Second * time.Duration(9))
		err := syncResourceTags()
		if err != nil {
			logger.Warning("timer: sync res tags fail:", err)
		}
	}
}

func syncResourceTags() error {
	start := time.Now()

	resources, err := models.ResourceGetAll()
	if err != nil {
		return err
	}

	resTagsMap := make(map[string]cache.ResourceAndTags)
	for i := 0; i < len(resources); i++ {
		tagslst := strings.Fields(resources[i].Tags)
		count := len(tagslst)
		if count == 0 {
			continue
		}

		tagsmap := make(map[string]string, count)
		for i := 0; i < count; i++ {
			arr := strings.Split(tagslst[i], "=")
			if len(arr) != 2 {
				continue
			}
			tagsmap[arr[0]] = arr[1]
		}

		resAndTags := cache.ResourceAndTags{
			Tags:     tagsmap,
			Resource: resources[i],
		}

		resTagsMap[resources[i].Ident] = resAndTags
	}

	cache.ResTags.SetAll(resTagsMap)
	logger.Debugf("timer: sync res tags done, cost: %dms", time.Since(start).Milliseconds())

	return nil
}
