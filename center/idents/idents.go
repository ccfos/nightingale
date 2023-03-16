package idents

import (
	"context"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"gorm.io/gorm"
)

type Set struct {
	sync.RWMutex
	items map[string]models.HostMeta
	db    *gorm.DB
	redis storage.Redis
}

func New(db *gorm.DB, redis storage.Redis) *Set {
	set := &Set{
		items: make(map[string]models.HostMeta),
		db:    db,
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
	now := time.Now().Unix()
	num := 0

	for _, meta := range items {
		m[meta.Hostname] = meta
		num++
		if num == 100 {
			if err := s.updateTargets(m, now); err != nil {
				logger.Errorf("failed to update targets: %v", err)
			}
			m = make(map[string]models.HostMeta, 100)
			num = 0
		}
	}

	if err := s.updateTargets(m, now); err != nil {
		logger.Errorf("failed to update targets: %v", err)
	}
}

func (s *Set) updateTargets(m map[string]models.HostMeta, now int64) error {
	count := int64(len(m))
	if count == 0 {
		return nil
	}

	var values []interface{}
	for ident, meta := range m {
		values = append(values, models.WrapIdent(ident))
		values = append(values, meta)
	}
	err := s.redis.MSet(context.Background(), values...).Err()
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

	err = s.db.Table("target").Where("ident in ?", exists).Update("update_at", now).Error
	if err != nil {
		logger.Error("failed to update target:", exists, "error:", err)
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
