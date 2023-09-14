package idents

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"gorm.io/gorm"
)

type Set struct {
	sync.Mutex
	items map[string]*TargetHeartbeat
	ctx   *ctx.Context
}
type TargetHeartbeat struct {
	HostIp string `json:"host_ip"`
}

func New(ctx *ctx.Context) *Set {
	set := &Set{
		items: make(map[string]*TargetHeartbeat),
		ctx:   ctx,
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
		s.items[ident] = nil
	}
}

func (s *Set) MSetTargetHeartbeat(items map[string]*TargetHeartbeat) {
	s.Lock()
	defer s.Unlock()
	for ident, thb := range items {
		s.items[ident] = &TargetHeartbeat{thb.HostIp}
	}
}

func (s *Set) LoopPersist() {
	for {
		time.Sleep(time.Second)
		s.persist()
	}
}

func (s *Set) persist() {
	var items map[string]*TargetHeartbeat

	s.Lock()
	if len(s.items) == 0 {
		s.Unlock()
		return
	}

	items = s.items
	s.items = make(map[string]*TargetHeartbeat)
	s.Unlock()

	s.updateTimestamp(items)
}

func (s *Set) updateTimestamp(items map[string]*TargetHeartbeat) {
	idents := make([]string, 0, 100)
	targetHeartbeats := make([]*TargetHeartbeat, 0, 100)
	now := time.Now().Unix()
	num := 0
	for ident, th := range items {
		idents = append(idents, ident)
		if th != nil {
			targetHeartbeats = append(targetHeartbeats, th)
		}
		num++
		if num == 100 {
			if len(targetHeartbeats) == 0 {
				targetHeartbeats = nil
			}
			if err := s.UpdateTargets(idents, targetHeartbeats, now); err != nil {
				logger.Errorf("failed to update targets: %v", err)
			}
			idents = idents[:0]
			targetHeartbeats = targetHeartbeats[:0]
			num = 0
		}
	}
	if len(targetHeartbeats) == 0 {
		targetHeartbeats = nil
	}
	if err := s.UpdateTargets(idents, targetHeartbeats, now); err != nil {
		logger.Errorf("failed to update targets: %v", err)
	}
}

type TargetUpdate struct {
	Idents           []string           `json:"Idents"`
	TargetHeartbeats []*TargetHeartbeat `json:"target_heartbeats"`
	Now              int64              `json:"now"`
}

// UpdateTargets updates a batch of target records in the database.
//
// It takes the following parameters:
//
// idents []string:
//   - A slice of target idents to update.
//
// targetHeartbeats []*TargetHeartbeat:
//   - A slice containing latest host IP info for the idents. Can be nil.
//
// now int64:
//   - The timestamp to set the update_at field to.
//
// If not the central node, it will send the update info to the central node.
//
// Otherwise, it checks the number of idents and returns early if none.
//
// It then performs the update based on whether targetHeartbeats is nil:
//
// - nil -> update only timestamps, from remote write path
// - non-nil -> update host IPs also, from heartbeat path
//
// After updating, it checks if any idents were not found, and inserts them.
//
// Returns any error encountered.
func (s *Set) UpdateTargets(idents []string, targetHeartbeats []*TargetHeartbeat, now int64) error {
	if !s.ctx.IsCenter {
		t := TargetUpdate{
			Idents:           idents,
			TargetHeartbeats: targetHeartbeats,
			Now:              now,
		}
		err := poster.PostByUrls(s.ctx, "/v1/n9e/target-update", t)
		return err
	}

	count := int64(len(idents))
	if count == 0 {
		return nil
	}
	var ret *gorm.DB
	if targetHeartbeats == nil {
		logger.Debugf("come from remote write. Idents = %+v", idents)
		ret = s.ctx.DB.Table("target").Where("ident in ?", idents).Update("update_at", now)
	} else {
		logger.Debugf("come from heartbeat. Idents = %+v,TargetHeartbeats = %+v", idents, targetHeartbeats)
		if len(idents) != len(targetHeartbeats) {
			return fmt.Errorf("invalid args len(Idents)= %v,len(targetHeartbeats)= %v", len(idents), len(targetHeartbeats))
		}
		ret = s.batchUpdateTargets(idents, targetHeartbeats, now)
	}

	if ret.Error != nil {
		return ret.Error
	}

	if ret.RowsAffected == count {
		return nil
	}

	// there are some Idents not found in db, so insert them
	var exists []string
	err := s.ctx.DB.Table("target").Where("ident in ?", idents).Pluck("ident", &exists).Error
	if err != nil {
		return err
	}

	news := slice.SubString(idents, exists)
	for i := 0; i < len(news); i++ {
		err = s.ctx.DB.Exec("INSERT INTO target(ident, update_at) VALUES(?, ?)", news[i], now).Error
		if err != nil {
			logger.Error("failed to insert target:", news[i], "error:", err)
		}
	}

	return nil
}

// batchUpdateTargets performs a batch update of target records in the database.
//
// It takes the following parameters:
//
// Idents []string:
//   - A slice of target Idents to update.
//
// targetHeartbeats []*TargetHeartbeat:
//   - A slice containing the latest host IP info for each ident.
//
// now int64:
//   - The timestamp to set the update_at field to.
//
// It starts a database transaction, and loops through the Idents performing an
// update on each one. The HostIp and UpdateAt fields are set based on the
// corresponding info in targetHeartbeats and now param.
//
// If any update fails, the transaction is rolled back.
//
// Returns the transaction handle to allow checking for errors.
func (s *Set) batchUpdateTargets(idents []string, targetHeartbeats []*TargetHeartbeat, now int64) *gorm.DB {
	tx := s.ctx.DB.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Error; err != nil {
		return tx
	}
	for i := range idents {
		targetNew := models.Target{HostIp: targetHeartbeats[i].HostIp, UpdateAt: now}
		tx.Model(&models.Target{}).Where("ident = ? ", idents[i]).Updates(targetNew)
		if err := tx.Error; err != nil {
			tx.Rollback()
			return tx
		}
	}
	defer func() {
		tx.RowsAffected = int64(len(idents))
	}()
	return tx.Commit()
}
