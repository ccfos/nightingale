package naming

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/storage"
)

// local servers
var localss string

type HeartbeatConfig struct {
	IP       string
	Interval int64
	Endpoint string
	Cluster  string
}

func Heartbeat(ctx context.Context, cfg HeartbeatConfig) error {
	if err := heartbeat(ctx, cfg); err != nil {
		fmt.Println("failed to heartbeat:", err)
		return err
	}

	go loopHeartbeat(ctx, cfg)
	return nil
}

func loopHeartbeat(ctx context.Context, cfg HeartbeatConfig) {
	interval := time.Duration(cfg.Interval) * time.Millisecond
	for {
		time.Sleep(interval)
		if err := heartbeat(ctx, cfg); err != nil {
			logger.Warning(err)
		}
	}
}

// hash struct:
// /server/heartbeat/Default -> {
//     10.2.3.4:19000 => $timestamp
//     10.2.3.5:19000 => $timestamp
// }
func redisKey(cluster string) string {
	return fmt.Sprintf("/server/heartbeat/%s", cluster)
}

func heartbeat(ctx context.Context, cfg HeartbeatConfig) error {
	now := time.Now().Unix()
	key := redisKey(cfg.Cluster)
	err := storage.Redis.HSet(ctx, key, cfg.Endpoint, now).Err()
	if err != nil {
		return err
	}

	servers, err := ActiveServers(ctx, cfg.Cluster)
	if err != nil {
		return err
	}

	sort.Strings(servers)
	newss := strings.Join(servers, " ")
	if newss != localss {
		RebuildConsistentHashRing(servers)
		localss = newss
	}

	return nil
}

func clearDeadServer(ctx context.Context, cluster, endpoint string) {
	key := redisKey(cluster)
	err := storage.Redis.HDel(ctx, key, endpoint).Err()
	if err != nil {
		logger.Warningf("failed to hdel %s %s, error: %v", key, endpoint, err)
	}
}

func ActiveServers(ctx context.Context, cluster string) ([]string, error) {
	ret, err := storage.Redis.HGetAll(ctx, redisKey(cluster)).Result()
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	dur := int64(20)

	actives := make([]string, 0, len(ret))
	for endpoint, clockstr := range ret {
		clock, err := strconv.ParseInt(clockstr, 10, 64)
		if err != nil {
			continue
		}

		if now-clock > dur {
			clearDeadServer(ctx, cluster, endpoint)
			continue
		}

		actives = append(actives, endpoint)
	}

	return actives, nil
}
