package dispatch

import (
	"encoding/json"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/toolkits/pkg/logger"
)

type EventRetryComsumer struct {
	ctx   *ctx.Context
	redis storage.Redis

	interval time.Duration
}

func NewEventRetryComsumer(ctx *ctx.Context, redis storage.Redis) *EventRetryComsumer {
	return &EventRetryComsumer{
		ctx:      ctx,
		redis:    redis,
		interval: 5 * time.Second,
	}
}

func (erc *EventRetryComsumer) Start() {
	if erc.ctx.IsCenter {
		return
	}

	go erc.loopComsume()
}

func (erc *EventRetryComsumer) loopComsume() {
	for {
		time.Sleep(100 * time.Millisecond)

		// 阻塞获取消息
		// result[0]: 队列名称
		// result[1]: 消息内容
		result, err := erc.redis.BLPop(erc.ctx.Ctx, 0, "failed_event_queue").Result()
		if err != nil {
			logger.Errorf("failed to pop from redis: %v", err)
			continue
		}

		// 检查消息格式
		if len(result) < 2 {
			logger.Errorf("failed to pop from redis: invalid result")
			continue
		}

		// 解析消息
		var event models.AlertCurEvent
		if err := json.Unmarshal([]byte(result[1]), &event); err != nil {
			// 解析失败，直接丢弃消息
			logger.Errorf("failed to unmarshal event: %v", err)
			continue
		}

		// 重新发送，直到成功
		for {
			var sendErr error
			if event.Id, sendErr = poster.PostByUrlsWithResp[int64](erc.ctx, "/v1/n9e/event-persist", &event); sendErr != nil {
				logger.Errorf("failed to send event: %v", sendErr)
				time.Sleep(erc.interval)
				continue
			}

			break
		}
	}
}