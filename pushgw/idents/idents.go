package idents

import (
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"gorm.io/gorm"
)

type Set struct {
	sync.Mutex
	items map[string]struct{}
	db    *gorm.DB
}

func New(db *gorm.DB) *Set {
	set := &Set{
		items: make(map[string]struct{}),
		db:    db,
	}

	set.Init()
	return set
}

func (s *Set) Init() {
	go s.LoopPersist()
}

func (s *Set) MSet(items map[string]struct{}) {
	s.Lock()
	defer s.Unlock()
	for ident := range items {
		s.items[ident] = struct{}{}
	}
}

func (s *Set) LoopPersist() {
	for {
		time.Sleep(time.Second)
		s.persist()
	}
}

func (s *Set) persist() {
	var items map[string]struct{}

	s.Lock()
	if len(s.items) == 0 {
		s.Unlock()
		return
	}

	items = s.items
	s.items = make(map[string]struct{})
	s.Unlock()

	s.updateTimestamp(items)
}

func (s *Set) updateTimestamp(items map[string]struct{}) {
	lst := make([]string, 0, 100)
	now := time.Now().Unix()
	num := 0
	for ident := range items {
		lst = append(lst, ident)
		num++
		if num == 100 {
			if err := s.updateTargets(lst, now); err != nil {
				logger.Errorf("failed to update targets: %v", err)
			}
			lst = lst[:0]
			num = 0
		}
	}

	if err := s.updateTargets(lst, now); err != nil {
		logger.Errorf("failed to update targets: %v", err)
	}
}

func (s *Set) updateTargets(lst []string, now int64) error {
	count := int64(len(lst))
	if count == 0 {
		return nil
	}

	ret := s.db.Table("target").Where("ident in ?", lst).Update("update_at", now)
	if ret.Error != nil {
		return ret.Error
	}

	if ret.RowsAffected == count {
		return nil
	}

	// there are some idents not found in db, so insert them
	var exists []string
	err := s.db.Table("target").Where("ident in ?", lst).Pluck("ident", &exists).Error
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
