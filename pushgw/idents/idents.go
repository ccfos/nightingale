package idents

// import (
// 	"math"
// 	"sync"
// 	"time"

// 	"github.com/toolkits/pkg/logger"
// 	"github.com/toolkits/pkg/slice"
// 	"gorm.io/gorm"
// )

// type Set struct {
// 	sync.Mutex
// 	items        map[string]int64
// 	db           *gorm.DB
// 	datasourceId int64
// 	maxOffset    int64
// }

// func New(db *gorm.DB, dsId, maxOffset int64) *Set {
// 	if maxOffset <= 0 {
// 		maxOffset = 500
// 	}
// 	set := &Set{
// 		items:        make(map[string]int64),
// 		db:           db,
// 		datasourceId: dsId,
// 		maxOffset:    maxOffset,
// 	}

// 	set.Init()
// 	return set
// }

// func (s *Set) Init() {
// 	go s.LoopPersist()
// }

// func (s *Set) MSet(items map[string]int64) {
// 	s.Lock()
// 	defer s.Unlock()
// 	for ident, ts := range items {
// 		s.items[ident] = ts
// 	}
// }

// func (s *Set) LoopPersist() {
// 	for {
// 		time.Sleep(time.Second)
// 		s.persist()
// 	}
// }

// func (s *Set) persist() {
// 	var items map[string]int64

// 	s.Lock()
// 	if len(s.items) == 0 {
// 		s.Unlock()
// 		return
// 	}

// 	items = s.items
// 	s.items = make(map[string]int64)
// 	s.Unlock()

// 	s.updateTimestamp(items)
// }

// func (s *Set) updateTimestamp(items map[string]int64) {
// 	lst := make([]string, 0, 100)
// 	offsetLst := make(map[string]int64)
// 	now := time.Now().Unix()
// 	num := 0

// 	largeOffsetTargets, _ := s.GetLargeOffsetTargets()

// 	for ident, ts := range items {
// 		// 和当前时间相差 maxOffset 毫秒以上的，更新偏移的时间
// 		// compare with current time, if offset is larger than maxOffset, update offset
// 		offset := int64(math.Abs(float64(ts - time.Now().UnixMilli())))
// 		if offset >= s.maxOffset {
// 			offsetLst[ident] = offset
// 		}

// 		// 如果是大偏移的，也更新时间
// 		// if offset is large, update timestamp
// 		if _, ok := largeOffsetTargets[ident]; ok {
// 			offsetLst[ident] = offset
// 		}

// 		lst = append(lst, ident)
// 		num++
// 		if num == 100 {
// 			if err := s.updateTargets(lst, now); err != nil {
// 				logger.Errorf("failed to update targets: %v", err)
// 			}
// 			lst = lst[:0]
// 			num = 0
// 		}
// 	}

// 	if err := s.updateTargets(lst, now); err != nil {
// 		logger.Errorf("failed to update targets: %v", err)
// 	}

// 	for ident, offset := range offsetLst {
// 		if err := s.updateTargetsAndOffset(ident, offset, now); err != nil {
// 			logger.Errorf("failed to update offset: %v", err)
// 		}
// 	}
// }

// func (s *Set) updateTargets(lst []string, now int64) error {
// 	count := int64(len(lst))
// 	if count == 0 {
// 		return nil
// 	}

// 	ret := s.db.Table("target").Where("ident in ?", lst).Update("update_at", now)
// 	if ret.Error != nil {
// 		return ret.Error
// 	}

// 	if ret.RowsAffected == count {
// 		return nil
// 	}

// 	// there are some idents not found in db, so insert them
// 	var exists []string
// 	err := s.db.Table("target").Where("ident in ?", lst).Pluck("ident", &exists).Error
// 	if err != nil {
// 		return err
// 	}

// 	news := slice.SubString(lst, exists)
// 	for i := 0; i < len(news); i++ {
// 		err = s.db.Exec("INSERT INTO target(ident, update_at, datasource_id) VALUES(?, ?, ?)", news[i], now, s.datasourceId).Error
// 		if err != nil {
// 			logger.Error("failed to insert target:", news[i], "error:", err)
// 		}
// 	}

// 	return nil
// }

// func (s *Set) updateTargetsAndOffset(ident string, offset, now int64) error {
// 	ret := s.db.Table("target").Where("ident = ?", ident).Updates(map[string]interface{}{"update_at": now, "offset": offset})
// 	if ret.Error != nil {
// 		return ret.Error
// 	}
// 	if ret.RowsAffected == 1 {
// 		return nil
// 	}

// 	// there are some idents not found in db, so insert them
// 	err := s.db.Exec("INSERT INTO target(ident, offset, update_at, datasource_id) VALUES(?, ?, ?, ?)", ident, offset, now, s.datasourceId).Error
// 	if err != nil {
// 		logger.Error("failed to insert target:", ident, "error:", err)
// 	}

// 	return nil
// }

// func (s *Set) GetLargeOffsetTargets() (map[string]struct{}, error) {
// 	var targets []string
// 	err := s.db.Table("target").Where("offset > ?", s.maxOffset).Pluck("ident", &targets).Error
// 	if err != nil {
// 		return nil, err
// 	}

// 	var m = make(map[string]struct{}, len(targets))
// 	for i := 0; i < len(targets); i++ {
// 		m[targets[i]] = struct{}{}
// 	}
// 	return m, nil
// }
