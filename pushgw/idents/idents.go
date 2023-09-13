package idents

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"gorm.io/gorm"
)

type Set struct {
	sync.Mutex
	items map[string]*TargetHeadBeat
	ctx   *ctx.Context
}
type TargetHeadBeat struct {
	HostIp string `json:"host_ip"`
}

func New(ctx *ctx.Context) *Set {
	set := &Set{
		items: make(map[string]*TargetHeadBeat),
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

// MSetTHB updates the internal items map with new host IP info.
//
// It takes in a map of ident -> TargetHeadBeat structs.
//
// The TargetHeadBeat struct contains the latest host IP for that ident.
//
// This allows efficiently updating the cached host IP mappings in one call.
//
// The items map will be locked during the update to prevent concurrent access.
//
func (s *Set) MSetTHB(items map[string]*TargetHeadBeat) {
	s.Lock()
	defer s.Unlock()
	for ident, thb := range items {
		s.items[ident] = &TargetHeadBeat{thb.HostIp}
	}
}

func (s *Set) LoopPersist() {
	for {
		time.Sleep(time.Second)
		s.persist()
	}
}

func (s *Set) persist() {
	var items map[string]*TargetHeadBeat

	s.Lock()
	if len(s.items) == 0 {
		s.Unlock()
		return
	}

	items = s.items
	s.items = make(map[string]*TargetHeadBeat)
	s.Unlock()

	s.updateTimestamp(items)
}

func (s *Set) updateTimestamp(items map[string]*TargetHeadBeat) {
	lst := make([]string, 0, 100)
	lsThb := make([]*TargetHeadBeat, 0, 100)
	now := time.Now().Unix()
	num := 0
	for ident, thb := range items {
		lst = append(lst, ident)
		if thb != nil {
			lsThb = append(lsThb, thb)
		}
		num++
		if num == 100 {
			if len(lsThb) == 0 {
				lsThb = nil
			}
			if err := s.UpdateTargets(lst, lsThb, now); err != nil {
				logger.Errorf("failed to update targets: %v", err)
			}
			lst = lst[:0]
			lsThb = lsThb[:0]
			num = 0
		}
	}
	if len(lsThb) == 0 {
		lsThb = nil
	}
	if err := s.UpdateTargets(lst, lsThb, now); err != nil {
		logger.Errorf("failed to update targets: %v", err)
	}
}

type TargetUpdate struct {
	Lst   []string          `json:"lst"`
	LsThb []*TargetHeadBeat `json:"ls_thb"`
	Now   int64             `json:"now"`
}

// UpdateTargets updates the targets in the database.
//
// It takes in:
//
//  - lst - a slice of target idents to update
//  - lsThb - a slice of TargetHeadBeat structs containing latest host IP info
//  - now - the timestamp to set the update_at field to
//
// If lsThb is nil, it will just update the timestamp for the idents in lst.
//
// Otherwise, it will do a batch update to set the host_ip and update_at from
// the info in lsThb.
//
// The batch update uses a SQL CASE statement for efficiency.
//
// If any idents in lst don't exist in the DB, it will insert the missing ones.
//
// Returns any error encountered.
func (s *Set) UpdateTargets(lst []string, lsThb []*TargetHeadBeat, now int64) error {
	if !s.ctx.IsCenter {
		t := TargetUpdate{
			Lst:   lst,
			LsThb: lsThb,
			Now:   now,
		}
		err := poster.PostByUrls(s.ctx, "/v1/n9e/target-update", t)
		return err
	}

	count := int64(len(lst))
	if count == 0 {
		return nil
	}
	var ret *gorm.DB
	if lsThb == nil {
		logger.Debugf("come from remote write. idents = %+v", lst)
		ret = s.ctx.DB.Table("target").Where("ident in ?", lst).Update("update_at", now)
	} else {
		logger.Debugf("come from heartbeat. idents = %+v,TargetHeadBeat = %+v", lst, lsThb)
		if len(lst) != len(lsThb) {
			return fmt.Errorf("invalid args len(lst)= %v,len(lsThb)= %v", len(lst), len(lsThb))
		}
		ret = s.batchUpdateTHB(lst, lsThb, now)
	}

	if ret.Error != nil {
		return ret.Error
	}

	if ret.RowsAffected == count {
		return nil
	}

	// there are some idents not found in db, so insert them
	var exists []string
	err := s.ctx.DB.Table("target").Where("ident in ?", lst).Pluck("ident", &exists).Error
	if err != nil {
		return err
	}

	news := slice.SubString(lst, exists)
	for i := 0; i < len(news); i++ {
		err = s.ctx.DB.Exec("INSERT INTO target(ident, update_at) VALUES(?, ?)", news[i], now).Error
		if err != nil {
			logger.Error("failed to insert target:", news[i], "error:", err)
		}
	}

	return nil
}

// batchUpdateTHB performs a batch update of targets in the database using host IP data from heartbeat messages.
// It generates a SQL CASE statement to efficiently update multiple rows in one query.
//
// The parameters are:
//   - lst: A slice of target idents to update
//   - lsThb: A slice of TargetHeadBeat structs containing the latest host IP data
//   - now: The timestamp to set the update_at field to
//
// The return value is the updated gorm DB struct to allow chaining further operations.
//
// This allows efficiently updating potentially many ident->IP mappings in one query instead
// of individual updates. The generated SQL will be like:
//
//   UPDATE target SET host_ip = CASE ident WHEN 'i1' THEN 'ip1' WHEN 'i2' THEN 'ip2' END, update_at = <now>
//   WHERE ident IN ('i1', 'i2', ...)
//
func (s *Set) batchUpdateTHB(lst []string, lsThb []*TargetHeadBeat, now int64) *gorm.DB {
	var b strings.Builder
	b.WriteString("UPDATE target SET host_ip = CASE ident ")
	for i := range lst {
		b.WriteString("WHEN '")
		b.WriteString(lst[i])
		b.WriteString("' THEN '")
		b.WriteString(lsThb[i].HostIp)
		b.WriteString("'")
	}
	b.WriteString("END, update_at = ? \nWHERE ident IN(?)")
	return s.ctx.DB.Exec(b.String(), now, lst)
}
