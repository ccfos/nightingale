package cache

import (
	"sync"

	"github.com/didi/nightingale/v5/models"
)

// resource_ident -> tags_map
// 监控数据上报的时候，要把资源的tags附到指标数据上
type ResTagsMap struct {
	sync.RWMutex
	Data map[string]ResourceAndTags
}

type ResourceAndTags struct {
	Tags     map[string]string
	Resource models.Resource
}

var ResTags = &ResTagsMap{Data: make(map[string]ResourceAndTags)}

func (r *ResTagsMap) SetAll(m map[string]ResourceAndTags) {
	r.Lock()
	defer r.Unlock()
	r.Data = m
}

func (r *ResTagsMap) Get(key string) (ResourceAndTags, bool) {
	r.RLock()
	defer r.RUnlock()

	value, exists := r.Data[key]

	return value, exists
}
