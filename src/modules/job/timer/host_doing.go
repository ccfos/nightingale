package timer

import (
	"fmt"
	"os"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/models"
)

// CacheHostDoing 缓存task_host_doing表全部内容，减轻DB压力
func CacheHostDoing() {
	err := cacheHostDoing()
	if err != nil {
		fmt.Println("cannot cache host_doing", err)
		os.Exit(1)
	}

	go loopCacheHostDoing()
}

func loopCacheHostDoing() {
	for {
		time.Sleep(time.Millisecond * 400)
		cacheHostDoing()
	}
}

func cacheHostDoing() error {
	lst, err := models.DoingHostList("")
	if err != nil {
		logger.Errorf("models.DoingHostList fail: %v", err)
		return err
	}

	cnt := len(lst)
	set := make(map[string][]models.TaskHostDoing, cnt)

	for i := 0; i < cnt; i++ {
		set[lst[i].Host] = append(set[lst[i].Host], lst[i])
	}

	models.SetDoingCache(set)
	return nil
}
