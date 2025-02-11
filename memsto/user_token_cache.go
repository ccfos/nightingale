package memsto

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type UserTokenCacheType struct {
	statTotal int64
	ctx       *ctx.Context
	stats     *Stats

	sync.RWMutex
	tokens         map[string]*models.User
	tokensLastUsed map[string]int64
}

func NewUserTokenCache(ctx *ctx.Context, stats *Stats) *UserTokenCacheType {
	utc := &UserTokenCacheType{
		statTotal:      -1,
		ctx:            ctx,
		stats:          stats,
		tokens:         make(map[string]*models.User),
		tokensLastUsed: make(map[string]int64),
	}
	utc.SyncUserTokens()
	return utc
}

func (utc *UserTokenCacheType) StatChanged(total int64) bool {
	if utc.statTotal == total {
		return false
	}
	return true
}

func (utc *UserTokenCacheType) Set(tokenUsers map[string]*models.User, total int64) {
	utc.Lock()
	utc.tokens = tokenUsers
	utc.Unlock()

	utc.statTotal = total
}

func (utc *UserTokenCacheType) GetByToken(token string) *models.User {
	utc.Lock()
	defer utc.Unlock()
	utc.tokensLastUsed[token] = time.Now().Unix()

	return utc.tokens[token]
}

func (utc *UserTokenCacheType) SyncUserTokens() {
	err := utc.syncUserTokens()
	if err != nil {
		fmt.Println("failed to sync user tokens:", err)
		exit(1)
	}

	go utc.loopSyncUserTokens()
	go utc.loopUpdateUserTokenLastUsedTime()
}

func (utc *UserTokenCacheType) loopUpdateUserTokenLastUsedTime() {
	duration := time.Duration(10) * time.Minute
	for {
		time.Sleep(duration)
		if err := utc.updateUserTokenLastUsedTime(); err != nil {
			logger.Warning("failed to update user token last used time:", err)
		}
	}
}

func (utc *UserTokenCacheType) loopSyncUserTokens() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := utc.syncUserTokens(); err != nil {
			logger.Warning("failed to sync user tokens:", err)
		}
	}
}

func (utc *UserTokenCacheType) updateUserTokenLastUsedTime() error {
	tokenLastUsedMap := make(map[string]int64)
	now := time.Now().Unix()

	utc.Lock()
	for token, lastUsedTime := range utc.tokensLastUsed {
		if lastUsedTime == 0 {
			continue
		}

		if now-lastUsedTime > 1800 {
			// 如果 token 已经 30 分钟没有使用，不再更新数据库
			delete(utc.tokensLastUsed, token)
			continue
		}

		tokenLastUsedMap[token] = lastUsedTime
	}
	utc.Unlock()

	for token, lastUsedTime := range tokenLastUsedMap {
		err := models.UserTokenUpdateLastUsedTime(utc.ctx, token, lastUsedTime)
		if err != nil {
			logger.Warning("failed to update user token last used time:", err)
			continue
		}
	}

	return nil
}

func (utc *UserTokenCacheType) syncUserTokens() error {
	start := time.Now()

	total, err := models.UserTokenTotal(utc.ctx)
	if err != nil {
		dumper.PutSyncRecord("user_tokens", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec UserTokenStatistics")
	}

	if !utc.StatChanged(total) {
		utc.stats.GaugeCronDuration.WithLabelValues("sync_user_tokens").Set(0)
		utc.stats.GaugeSyncNumber.WithLabelValues("sync_user_tokens").Set(0)
		dumper.PutSyncRecord("user_tokens", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.UserTokenGetAll(utc.ctx)
	if err != nil {
		dumper.PutSyncRecord("user_tokens", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to exec UserTokenGetAll")
	}

	users, err := models.UserGetAll(utc.ctx)
	if err != nil {
		dumper.PutSyncRecord("user_tokens", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to exec UserGetAll")
	}

	userMap := make(map[string]*models.User)
	for _, user := range users {
		userMap[user.Username] = user
	}

	tokenUsers := make(map[string]*models.User)
	for _, token := range lst {
		user, ok := userMap[token.Username]
		if !ok {
			continue
		}

		tokenUsers[token.Token] = user
	}

	utc.Set(tokenUsers, total)

	ms := time.Since(start).Milliseconds()
	utc.stats.GaugeCronDuration.WithLabelValues("sync_user_tokens").Set(float64(ms))
	utc.stats.GaugeSyncNumber.WithLabelValues("sync_user_tokens").Set(float64(len(tokenUsers)))

	logger.Infof("timer: sync user tokens done, cost: %dms, number: %d", ms, len(tokenUsers))
	dumper.PutSyncRecord("user_tokens", start.Unix(), ms, len(tokenUsers), "success")

	return nil
}
