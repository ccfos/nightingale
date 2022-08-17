package naming

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/stat"
	"github.com/didi/nightingale/v5/src/storage"
)

// local servers
var localss string

func Heartbeat(ctx context.Context) error {
	if err := heartbeat(ctx); err != nil {
		stat.ReportError(stat.RedisOperateError)
		fmt.Println("failed to heartbeat:", err)
		return err
	}

	go loopHeartbeat(ctx)
	return nil
}

func loopHeartbeat(ctx context.Context) {
	interval := time.Duration(config.C.Heartbeat.Interval) * time.Millisecond
	for {
		time.Sleep(interval)
		if err := heartbeat(ctx); err != nil {
			logger.Warning(err)
		}
	}
}

// hash struct:
//
//	/server/heartbeat/Default -> {
//	    10.2.3.4:19000 => $timestamp
//	    10.2.3.5:19000 => $timestamp
//	}
func redisKey(cluster string) string {
	return fmt.Sprintf("/server/heartbeat/%s", cluster)
}

func heartbeat(ctx context.Context) error {
	now := time.Now().Unix()
	key := redisKey(config.C.ClusterName)
	err := storage.Redis.HSet(ctx, key, config.C.Heartbeat.Endpoint, now).Err()
	if err != nil {
		stat.ReportError(stat.RedisOperateError)
		return err
	}

	servers, err := ActiveServers(ctx, config.C.ClusterName)
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
		stat.ReportError(stat.RedisOperateError)
		logger.Warningf("failed to hdel %s %s, error: %v", key, endpoint, err)
	}
}

func ActiveServers(ctx context.Context, cluster string) ([]string, error) {
	ret, err := storage.Redis.HGetAll(ctx, redisKey(cluster)).Result()
	if err != nil {
		stat.ReportError(stat.RedisOperateError)
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
