package models

import "sync"

type TaskHostDoing struct {
	Id     int64
	Host   string
	Clock  int64
	Action string
}

func DoingHostList(where string, args ...interface{}) ([]TaskHostDoing, error) {
	var objs []TaskHostDoing
	err := DB["job"].Where(where, args...).Find(&objs)
	return objs, err
}

func DoingHostCount(where string, args ...interface{}) (int64, error) {
	return DB["job"].Where(where, args...).Count(new(TaskHostDoing))
}

var (
	doingLock sync.RWMutex
	doingMaps map[string][]TaskHostDoing
)

func SetDoingCache(v map[string][]TaskHostDoing) {
	doingLock.Lock()
	doingMaps = v
	doingLock.Unlock()
}

func GetDoingCache(k string) []TaskHostDoing {
	doingLock.RLock()
	defer doingLock.RUnlock()
	return doingMaps[k]
}
