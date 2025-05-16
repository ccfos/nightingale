package idents

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
)

type Set struct {
	sync.Mutex
	items   map[string]struct{}
	redis   storage.Redis
	ctx     *ctx.Context
	configs pconf.Pushgw
}

func New(ctx *ctx.Context, redis storage.Redis, configs pconf.Pushgw) *Set {
	set := &Set{
		items:   make(map[string]struct{}),
		redis:   redis,
		ctx:     ctx,
		configs: configs,
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
			if err := s.UpdateTargets(lst, now); err != nil {
				logger.Errorf("failed to update targets: %v", err)
			}
			lst = lst[:0]
			num = 0
		}
	}

	if err := s.UpdateTargets(lst, now); err != nil {
		logger.Errorf("failed to update targets: %v", err)
	}
}

type TargetUpdate struct {
	Lst []string `json:"lst"`
	Now int64    `json:"now"`
}

func (s *Set) UpdateTargets(lst []string, now int64) error {
	if len(lst) == 0 {
		return nil
	}

	err := s.updateTargetsUpdateTs(lst, now, s.redis)
	if err != nil {
		logger.Errorf("update_ts: failed to update targets: %v error: %v", lst, err)
	}

	if !s.ctx.IsCenter {
		t := TargetUpdate{
			Lst: lst,
			Now: now,
		}
		err := poster.PostByUrls(s.ctx, "/v1/n9e/target-update", t)
		return err
	}

	// there are some idents not found in db, so insert them
	var exists []string
	err = s.ctx.DB.Table("target").Where("ident in ?", lst).Pluck("ident", &exists).Error
	if err != nil {
		return err
	}

	news := slice.SubString(lst, exists)
	for i := 0; i < len(news); i++ {
		err = s.ctx.DB.Exec("INSERT INTO target(ident, update_at) VALUES(?, ?)", news[i], now).Error
		if err != nil {
			logger.Error("upsert_target: failed to insert target:", news[i], "error:", err)
		}
	}

	// 从批量更新一批机器的时间戳，改成逐台更新，是为了避免批量更新时，mysql的锁竞争问题
	for i := 0; i < len(exists); i++ {
		err = s.ctx.DB.Exec("UPDATE target SET update_at = ? WHERE ident = ?", now, exists[i]).Error
		if err != nil {
			logger.Error("upsert_target: failed to update target:", exists[i], "error:", err)
		}
	}

	return nil
}

func (s *Set) updateTargetsUpdateTs(lst []string, now int64, redis storage.Redis) error {
	if redis == nil {
		return fmt.Errorf("redis is nil")
	}

	newMap := make(map[string]interface{}, len(lst))
	for _, ident := range lst {
		hostUpdateTime := models.HostUpdteTime{
			UpdateTime: now,
			Ident:      ident,
		}
		newMap[models.WrapIdentUpdateTime(ident)] = hostUpdateTime
	}

	return s.updateTargetTsInRedis(newMap, redis)
}

func (s *Set) updateTargetTsInRedis(newMap map[string]interface{}, redis storage.Redis) (err error) {
	if len(newMap) == 0 {
		return nil
	}

	timeout := time.Duration(s.configs.UpdateTargetTimeoutMills) * time.Millisecond
	batchSize := s.configs.UpdateTargetBatchSize

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if len(newMap) <= batchSize {
		// 如果 newMap 的内容小于等于 batchSize，则直接执行 MSet
		return s.writeTargetTsInRedis(ctx, redis, newMap)
	}

	i := 0
	batchMap := make(map[string]interface{}, batchSize)
	for mapKey := range newMap {
		batchMap[mapKey] = newMap[mapKey]
		if (i+1)%batchSize == 0 {
			if e := s.writeTargetTsInRedis(ctx, redis, batchMap); e != nil {
				err = e
			}
			batchMap = make(map[string]interface{}, batchSize)
		}
		i++
	}
	if len(batchMap) > 0 {
		if e := s.writeTargetTsInRedis(ctx, redis, batchMap); e != nil {
			err = e
		}
	}

	return err
}

func (s *Set) writeTargetTsInRedis(ctx context.Context, redis storage.Redis, content map[string]interface{}) error {
	retryCount := s.configs.UpdateTargetRetryCount
	retryInterval := time.Duration(s.configs.UpdateTargetRetryIntervalMills) * time.Millisecond

	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}

	for i := 0; i < retryCount; i++ {
		start := time.Now()
		err := storage.MSet(ctx, redis, content)
		duration := time.Since(start).Seconds()

		logger.Debugf("update_ts: write target ts in redis, keys: %v, retryCount: %d, retryInterval: %v, error: %v", keys, retryCount, retryInterval, err)
		if err == nil {
			pstat.RedisOperationLatency.WithLabelValues("mset_target_ts", "success").Observe(duration)
			return nil
		} else {
			logger.Errorf("update_ts: failed to write target ts in redis: %v, keys: %v, retry %d/%d", err, keys, i+1, retryCount)
		}

		if i < retryCount-1 {
			// 最后一次尝试的时候不需要 sleep，之前的尝试如果失败了，都需要完事之后 sleep
			time.Sleep(retryInterval)
		}

		if i == retryCount-1 {
			// 记录最后一次的失败情况
			pstat.RedisOperationLatency.WithLabelValues("mset_target_ts", "fail").Observe(duration)
		}
	}

	return fmt.Errorf("failed to write target ts in redis after %d retries, keys: %v", retryCount, keys)
}
