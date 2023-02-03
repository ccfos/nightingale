package sender

import (
	"context"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/storage"
)

func PublishToRedis(clusterName string, bs []byte) {
	if len(bs) == 0 {
		return
	}
	if !config.C.Alerting.RedisPub.Enable {
		return
	}

	// pub all alerts to redis
	channelKey := config.C.Alerting.RedisPub.ChannelPrefix + clusterName
	err := storage.Redis.Publish(context.Background(), channelKey, bs).Err()
	if err != nil {
		logger.Errorf("event_notify: redis publish %s err: %v", channelKey, err)
	}
}
