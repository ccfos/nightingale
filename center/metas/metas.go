package metas

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/center/cstats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/toolkits/pkg/logger"
)

type Set struct {
	sync.RWMutex
	items map[string]models.HostMeta
	redis storage.Redis
}

func New(redis storage.Redis) *Set {
	set := &Set{
		items: make(map[string]models.HostMeta),
		redis: redis,
	}

	set.Init()
	return set
}

func (s *Set) Init() {
	go s.LoopPersist()
}

func (s *Set) MSet(items map[string]models.HostMeta) {
	s.Lock()
	defer s.Unlock()
	for ident, meta := range items {
		s.items[ident] = meta
	}
}

func (s *Set) Set(ident string, meta models.HostMeta) {
	s.Lock()
	defer s.Unlock()
	s.items[ident] = meta
}

func (s *Set) LoopPersist() {
	for {
		time.Sleep(time.Second)
		s.persist()
	}
}

func (s *Set) persist() {
	var items map[string]models.HostMeta

	s.Lock()
	if len(s.items) == 0 {
		s.Unlock()
		return
	}

	items = s.items
	s.items = make(map[string]models.HostMeta)
	s.Unlock()

	s.updateMeta(items)
}

func (s *Set) updateMeta(items map[string]models.HostMeta) {
	m := make(map[string]models.HostMeta, 100)
	num := 0

	for _, meta := range items {
		m[meta.Hostname] = meta
		num++
		if num == 100 {
			if err := s.updateTargets(m); err != nil {
				logger.Errorf("failed to update targets: %v", err)
			}
			m = make(map[string]models.HostMeta, 100)
			num = 0
		}
	}

	if err := s.updateTargets(m); err != nil {
		logger.Errorf("failed to update targets: %v", err)
	}
}

func (s *Set) updateTargets(m map[string]models.HostMeta) error {
	if s.redis == nil {
		logger.Warningf("redis is nil")
		return nil
	}

	count := int64(len(m))
	if count == 0 {
		return nil
	}

	newMap := make(map[string]interface{}, count)
	extendMap := make(map[string]interface{})
	for ident, meta := range m {
		if meta.ExtendInfo != nil {
			extendMeta := meta.ExtendInfo
			meta.ExtendInfo = make(map[string]interface{})
			extendMetaStr, err := json.Marshal(extendMeta)
			if err != nil {
				return err
			}
			extendMap[models.WrapExtendIdent(ident)] = extendMetaStr
		}
		newMap[models.WrapIdent(ident)] = meta
	}

	start := time.Now()
	err := storage.MSet(context.Background(), s.redis, newMap, 7*24*time.Hour)
	if err != nil {
		cstats.RedisOperationLatency.WithLabelValues("mset_target_meta", "fail").Observe(time.Since(start).Seconds())
		return err
	} else {
		cstats.RedisOperationLatency.WithLabelValues("mset_target_meta", "success").Observe(time.Since(start).Seconds())
	}

	if len(extendMap) > 0 {
		err = storage.MSet(context.Background(), s.redis, extendMap, 7*24*time.Hour)
		if err != nil {
			cstats.RedisOperationLatency.WithLabelValues("mset_target_extend", "fail").Observe(time.Since(start).Seconds())
			return err
		} else {
			cstats.RedisOperationLatency.WithLabelValues("mset_target_extend", "success").Observe(time.Since(start).Seconds())
		}
	}

	return err
}
