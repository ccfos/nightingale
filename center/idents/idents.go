package idents

import (
	"context"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/storage"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"gorm.io/gorm"
)

type HostMeta struct {
	AgentVersion string  `json:"agent_version"`
	Os           string  `json:"os"`
	Arch         string  `json:"arch"`
	Hostname     string  `json:"hostname"`
	CpuNum       int     `json:"cpu_num"`
	CpuUtil      float64 `json:"cpu_util"`
	MemUtil      float64 `json:"mem_util"`
	Offset       int64   `json:"offset"`
	UnixTime     int64   `json:"unixtime"`
}

type Set struct {
	sync.RWMutex
	items        map[string]HostMeta
	db           *gorm.DB
	redis        storage.Redis
	datasourceId int64
	maxOffset    int64
}

func New(db *gorm.DB, redis storage.Redis, dsId, maxOffset int64) *Set {
	if maxOffset <= 0 {
		maxOffset = 500
	}
	set := &Set{
		items:        make(map[string]HostMeta),
		db:           db,
		redis:        redis,
		datasourceId: dsId,
		maxOffset:    maxOffset,
	}

	set.Init()
	return set
}

func (s *Set) Init() {
	go s.LoopPersist()
}

func (s *Set) MSet(items map[string]HostMeta) {
	s.Lock()
	defer s.Unlock()
	for ident, meta := range items {
		s.items[ident] = meta
	}
}

func (s *Set) Set(ident string, meta HostMeta) {
	s.Lock()
	defer s.Unlock()
	s.items[ident] = meta
}

func (s *Set) Get(ident string) (HostMeta, bool) {
	s.RLock()
	defer s.RUnlock()
	meta, exists := s.items[ident]
	return meta, exists
}

func (s *Set) LoopPersist() {
	for {
		time.Sleep(time.Second)
		s.persist()
	}
}

func (s *Set) persist() {
	var items map[string]HostMeta

	s.Lock()
	if len(s.items) == 0 {
		s.Unlock()
		return
	}

	items = s.items
	s.items = make(map[string]HostMeta)
	s.Unlock()

	s.updateMeta(items)
}

func (s *Set) updateMeta(items map[string]HostMeta) {
	m := make(map[string]HostMeta, 100)
	now := time.Now().Unix()
	num := 0

	for _, meta := range items {
		m[meta.Hostname] = meta
		num++
		if num == 100 {
			if err := s.updateTargets(m, now); err != nil {
				logger.Errorf("failed to update targets: %v", err)
			}
			m = make(map[string]HostMeta, 100)
			num = 0
		}
	}

	if err := s.updateTargets(m, now); err != nil {
		logger.Errorf("failed to update targets: %v", err)
	}
}

func (s *Set) updateTargets(m map[string]HostMeta, now int64) error {
	count := int64(len(m))
	if count == 0 {
		return nil
	}

	// 一次性更新所有的 ident 写到 redis 中
	err := s.redis.MSet(context.Background(), m).Err()
	if err != nil {
		return err
	}
	var lst []string
	for ident := range m {
		lst = append(lst, ident)
	}

	// there are some idents not found in db, so insert them
	var exists []string
	err = s.db.Table("target").Where("ident in ?", lst).Pluck("ident", &exists).Error
	if err != nil {
		return err
	}

	news := slice.SubString(lst, exists)
	for i := 0; i < len(news); i++ {
		err = s.db.Exec("INSERT INTO target(ident, update_at) VALUES(?, ?)", news[i], now).Error
		if err != nil {
			logger.Error("failed to insert target:", news[i], "error:", err)
		}
	}

	return nil
}
