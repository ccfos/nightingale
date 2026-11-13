package router

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func targetReadRedis(t *testing.T, rt *Router) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	rt.Redis = client
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return server
}

func TestTargetGetsByHostFilter_ReturnsOnlySelectedHosts(t *testing.T) {
	rt := targetMutationRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	seedTargetMutationHosts(t, rt)
	targetReadRedis(t, rt)
	form := TargetQuery{
		Filters: []models.HostQuery{{Key: "hosts", Op: "==", Values: []interface{}{"host-a"}}}, P: 1, Limit: 10,
	}

	w := identityRequest(t, rt, rt.targetGetsByHostFilter, form, "0", nil)

	var got struct {
		List  []models.Target `json:"list"`
		Total int64           `json:"total"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.List, 1)
	require.Equal(t, "host-a", got.List[0].Ident)
	require.Equal(t, "10.0.0.1", got.List[0].HostIp)
}

func TestTargetGetTags_RespectsSelectionAndHostTagOption(t *testing.T) {
	rt := targetMutationRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Target{
		{Ident: "host-a", Tags: "env=prod", HostTags: []string{"os=linux"}},
		{Ident: "host-b", Tags: "env=stage", HostTags: []string{"os=linux"}},
		{Ident: "foreign", Tags: "private=yes"},
	}).Error)

	w := identityRequest(t, rt, rt.targetGetTags, nil, "0?idents=host-a,host-b&ignore_host_tag=true", nil)

	var got []string
	require.Empty(t, identityReply(t, w, &got))
	require.ElementsMatch(t, []string{"env=prod", "env=stage"}, got)
	withHost := identityRequest(t, rt, rt.targetGetTags, nil, "0?idents=host-a,host-b", nil)
	require.Empty(t, identityReply(t, withHost, &got))
	require.ElementsMatch(t, []string{"env=prod", "env=stage", "os=linux"}, got)
}

func TestDowntimeFilter_SelectsMatchingHostsWithEitherSQLStrategy(t *testing.T) {
	cases := []struct {
		name      string
		downtime  int64
		beatTimes []int64
		expected  []string
	}{
		{name: "none", downtime: 300, beatTimes: []int64{1 << 62, 1 << 62, 1 << 62}},
		{name: "all", downtime: 300, beatTimes: []int64{1, 1, 1}, expected: []string{"a", "b", "c"}},
		{name: "inclusion", downtime: 300, beatTimes: []int64{1, 1 << 62, 1 << 62}, expected: []string{"a"}},
		{name: "exclusion", downtime: 300, beatTimes: []int64{1, 1, 1 << 62}, expected: []string{"a", "b"}},
		{name: "recent", downtime: -300, beatTimes: []int64{1 << 62, 1, 1}, expected: []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := targetMutationRouter(t, &models.Target{}, &models.TargetBusiGroup{})
			targets := make(map[string]*models.Target)
			for i, ident := range []string{"a", "b", "c"} {
				target := &models.Target{Ident: ident}
				require.NoError(t, rt.Ctx.DB.Create(target).Error)
				target.BeatTime = tc.beatTimes[i]
				targets[ident] = target
			}
			rt.TargetCache = &memsto.TargetCacheType{}
			rt.TargetCache.Set(targets, 3, 1)

			option, matched := rt.downtimeFilter(tc.downtime)

			require.Equal(t, len(tc.expected) > 0, matched)
			if !matched {
				return
			}
			var options []models.BuildTargetWhereOption
			if option != nil {
				options = append(options, option)
			}
			hosts, err := models.TargetGets(rt.Ctx, 10, 0, "ident", false, options...)
			require.NoError(t, err)
			actual := make([]string, 0, len(hosts))
			for _, host := range hosts {
				actual = append(actual, host.Ident)
			}
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestTargetExtendInfoByIdent_ReturnsStoredMetadata(t *testing.T) {
	rt := &Router{}
	targetReadRedis(t, rt)
	require.NoError(t, rt.Redis.Set(context.Background(), models.WrapExtendIdent("host-a"), `{"rack":"r1"}`, 0).Err())

	w := identityRequest(t, rt, rt.targetExtendInfoByIdent, nil, "0?ident=host-a", nil)

	var got map[string]string
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, map[string]string{"ident": "host-a", "extend_info": `{"rack":"r1"}`}, got)
}

func TestTargetGets_DowntimeWithoutMatchesReturnsEmptyPage(t *testing.T) {
	rt := targetMutationRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	rt.TargetCache = &memsto.TargetCacheType{}
	rt.TargetCache.Set(map[string]*models.Target{"a": {Ident: "a", BeatTime: 1 << 62}}, 1, 1)

	w := identityRequest(t, rt, rt.targetGets, nil, "0?downtime=300", &models.User{RolesLst: []string{models.AdminRole}})

	var got struct {
		List  []models.Target `json:"list"`
		Total int64           `json:"total"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Empty(t, got.List)
	require.NotNil(t, got.List)
	require.Zero(t, got.Total)
}
