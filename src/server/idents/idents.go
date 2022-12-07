package idents

import (
	"context"
	"fmt"
	"strconv"
	"time"

	cmap "github.com/orcaman/concurrent-map"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/common"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/didi/nightingale/v5/src/server/naming"
	"github.com/didi/nightingale/v5/src/server/writer"
	"github.com/didi/nightingale/v5/src/storage"
)

// ident -> timestamp
var Idents = cmap.New()

func loopToRedis(ctx context.Context) {
	duration := time.Duration(4) * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(duration):
			toRedis()
		}
	}
}

func toRedis() {
	items := Idents.Items()
	if len(items) == 0 {
		return
	}

	if config.ReaderClients.IsNil(config.C.ClusterName) {
		return
	}

	now := time.Now().Unix()

	// clean old idents
	for key, at := range items {
		if at.(int64) < now-config.C.NoData.Interval {
			Idents.Remove(key)
		} else {
			// use now as timestamp to redis
			err := storage.Redis.HSet(context.Background(), redisKey(config.C.ClusterName), key, now).Err()
			if err != nil {
				logger.Errorf("redis hset idents failed: %v", err)
			}
		}
	}
}

// hash struct:
// /idents/Default -> {
//     $ident => $timestamp
//     $ident => $timestamp
// }
func redisKey(cluster string) string {
	return fmt.Sprintf("/idents/%s", cluster)
}

func clearDeadIdent(ctx context.Context, cluster, ident string) {
	key := redisKey(cluster)
	err := storage.Redis.HDel(ctx, key, ident).Err()
	if err != nil {
		logger.Warningf("failed to hdel %s %s, error: %v", key, ident, err)
	}
}

func Handle(ctx context.Context) {
	go loopToRedis(ctx)
	go loopPushMetrics(ctx)
}

func loopPushMetrics(ctx context.Context) {
	duration := time.Duration(10) * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(duration):
			pushMetrics()
		}
	}
}

func pushMetrics() {
	clusterName := config.C.ClusterName
	isLeader, err := naming.IamLeader(clusterName)
	if err != nil {
		logger.Errorf("handle_idents: %v", err)
		return
	}

	if !isLeader {
		logger.Info("handle_idents: i am not leader")
		return
	}

	// get all the target heartbeat timestamp
	ret, err := storage.Redis.HGetAll(context.Background(), redisKey(clusterName)).Result()
	if err != nil {
		logger.Errorf("handle_idents: redis hgetall fail: %v", err)
		return
	}

	now := time.Now().Unix()
	dur := config.C.NoData.Interval

	actives := make(map[string]struct{})
	for ident, clockstr := range ret {
		clock, err := strconv.ParseInt(clockstr, 10, 64)
		if err != nil {
			continue
		}

		if now-clock > dur {
			clearDeadIdent(context.Background(), clusterName, ident)
		} else {
			actives[ident] = struct{}{}
		}
	}

	// 有心跳，target_up = 1
	// 如果找到target，就把target的tags补充到series上
	// 如果没有target，就在数据库创建target
	for active := range actives {
		// build metrics
		pt := &prompb.TimeSeries{}
		pt.Samples = append(pt.Samples, prompb.Sample{
			// use ms
			Timestamp: now * 1000,
			Value:     1,
		})

		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  model.MetricNameLabel,
			Value: config.C.NoData.Metric,
		})

		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  "ident",
			Value: active,
		})

		target, has := memsto.TargetCache.Get(active)
		if !has {
			// target not exists
			target = &models.Target{
				Cluster:  clusterName,
				Ident:    active,
				Tags:     "",
				TagsJSON: []string{},
				TagsMap:  make(map[string]string),
				UpdateAt: now,
			}

			if err := target.Add(); err != nil {
				logger.Errorf("handle_idents: insert target(%s) fail: %v", active, err)
			}
		} else {
			common.AppendLabels(pt, target)
		}

		writer.Writers.PushSample("target_up", pt)
	}

	// 把actives传给TargetCache，看看除了active的部分，还有别的target么？有的话返回，设置target_up = 0
	deads := memsto.TargetCache.GetDeads(actives)
	for ident, dead := range deads {
		if ident == "" {
			continue
		}
		// build metrics
		pt := &prompb.TimeSeries{}
		pt.Samples = append(pt.Samples, prompb.Sample{
			// use ms
			Timestamp: now * 1000,
			Value:     0,
		})

		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  model.MetricNameLabel,
			Value: config.C.NoData.Metric,
		})

		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  "ident",
			Value: ident,
		})

		common.AppendLabels(pt, dead)
		writer.Writers.PushSample("target_up", pt)
	}
}
