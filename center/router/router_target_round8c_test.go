package router

import (
	"fmt"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestR8cTargetGroupResetRequiresForceWhenBlocked(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprint(force), func(t *testing.T) {
			rt := r8cRouter(t, &models.Target{}, &models.TargetBusiGroup{})
			require.NoError(t, rt.Ctx.DB.Create(&models.Target{Ident: "host"}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.TargetBusiGroup{TargetIdent: "host", GroupId: 1}).Error)
			rt.TargetBgidChangeCheck = func(ids []string, action string, bgids []int64) (map[string]string, error) {
				require.Equal(t, []string{"host"}, ids)
				require.Equal(t, "reset", action)
				require.Equal(t, []int64{2}, bgids)
				return map[string]string{"host": "used by collector"}, nil
			}
			var result map[string]string
			r8cData(t, r8cRequest(t, rt.targetBindBgids, targetBgidsForm{Idents: []string{"host"}, Bgids: []int64{2}, Action: "reset", Force: force}, "/1"), &result)
			groups, err := models.TargetGroupIdsGetByIdent(rt.Ctx, "host")
			require.NoError(t, err)
			if force {
				require.Empty(t, result)
				require.Equal(t, []int64{2}, groups)
			} else {
				require.Equal(t, "used by collector", result["host"])
				require.Equal(t, []int64{1}, groups)
			}
		})
	}
}
func TestR8cTargetHeartbeatClassification(t *testing.T) {
	rt := r8cRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	mr := targetReadRedis(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Target{{Ident: "recent"}, {Ident: "aging"}}).Error)
	now := time.Now().Unix()
	mr.Set(models.WrapIdentUpdateTime("recent"), fmt.Sprintf(`{"ident":"recent","update_time":%d}`, now))
	mr.Set(models.WrapIdentUpdateTime("aging"), fmt.Sprintf(`{"ident":"aging","update_time":%d}`, now-90))
	var result struct {
		List  []models.Target `json:"list"`
		Total int             `json:"total"`
	}
	r8cData(t, r8cRequest(t, rt.targetGetsByHostFilter, TargetQuery{P: 1, Limit: 10}, "/1"), &result)
	require.Equal(t, 2, result.Total)
	state := map[string]int{}
	for _, host := range result.List {
		state[host.Ident] = int(host.TargetUp)
	}
	require.Equal(t, 2, state["recent"])
	require.Equal(t, 1, state["aging"])
}
func TestR8cTargetStatsUtilizationBuckets(t *testing.T) {
	rt := &Router{TargetCache: &memsto.TargetCacheType{}}
	rt.TargetCache.Set(map[string]*models.Target{"one": {Ident: "one", CpuNum: 4, CpuUtil: 30, MemUtil: 70}, "two": {Ident: "two", CpuNum: 4, CpuUtil: 50, MemUtil: 90}}, 2, 1)
	var data struct {
		Count  int            `json:"count"`
		CPU    map[string]int `json:"cpu_usage"`
		Memory map[string]int `json:"mem_usage"`
	}
	r8cData(t, r8cRequest(t, rt.targetStats, nil, "/1"), &data)
	require.Equal(t, 2, data.Count)
	require.Equal(t, 1, data.CPU["40"])
	require.Equal(t, 1, data.CPU["60"])
	require.Equal(t, 1, data.Memory["80"])
}
