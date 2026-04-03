package stream

import (
	"context"
	"time"

	"github.com/ccfos/nightingale/v6/storage"
	"github.com/redis/go-redis/v9"
	"github.com/toolkits/pkg/logger"
)

const renewLeaseScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("EXPIRE", KEYS[1], tonumber(ARGV[2]))
end
return 0
`

// LeaderLease 多副本下同一 AppKey 仅一个实例持有，用于启动 Stream。
type LeaderLease struct {
	rdb    storage.Redis
	key    string
	holder string
	ttl    time.Duration
	stopCh chan struct{}
}

func NewLeaderLease(rdb storage.Redis, key, holder string, ttl time.Duration) *LeaderLease {
	if ttl < 5*time.Second {
		ttl = 30 * time.Second
	}
	return &LeaderLease{rdb: rdb, key: key, holder: holder, ttl: ttl, stopCh: make(chan struct{})}
}

// TryAcquire SET NX EX；返回是否拿到租约。
func (l *LeaderLease) TryAcquire(ctx context.Context) (bool, error) {
	if l.rdb == nil || l.key == "" || l.holder == "" {
		return false, nil
	}
	ok, err := l.rdb.SetNX(ctx, l.key, l.holder, l.ttl).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

// StartRenew 周期性续期，直到 stopCh 关闭。
func (l *LeaderLease) StartRenew(ctx context.Context) {
	interval := l.ttl / 3
	if interval < 2*time.Second {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.renewOnce(ctx)
		}
	}
}

func (l *LeaderLease) renewOnce(ctx context.Context) {
	if l.rdb == nil {
		return
	}
	res, err := l.rdb.Eval(ctx, renewLeaseScript, []string{l.key}, l.holder, int(l.ttl.Seconds())).Int()
	if err != nil {
		logger.Warningf("dingtalk stream lease renew failed key=%s: %v", l.key, err)
		return
	}
	if res != 1 {
		logger.Warningf("dingtalk stream lease lost key=%s", l.key)
	}
}

// StopRenew 停止续期协程。
func (l *LeaderLease) StopRenew() {
	select {
	case <-l.stopCh:
	default:
		close(l.stopCh)
	}
}

// Release 仅删除自己持有的锁。
func (l *LeaderLease) Release(ctx context.Context) {
	if l.rdb == nil || l.key == "" {
		return
	}
	val, err := l.rdb.Get(ctx, l.key).Result()
	if err == redis.Nil || err != nil {
		return
	}
	if val != l.holder {
		return
	}
	if err := l.rdb.Del(ctx, l.key).Err(); err != nil {
		logger.Warningf("dingtalk stream lease release del failed: %v", err)
	}
}
