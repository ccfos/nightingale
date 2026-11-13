package memsto

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestRuleAndTemplateCacheLifecycle(t *testing.T) {
	// Each cache must replace deleted entries and invalidate on either stat component.
	alert := &AlertRuleCacheType{}
	notify := &NotifyRuleCacheType{}
	record := &RecordingRuleCacheType{}
	pipeline := &EventProcessorCacheType{}
	tpl := &MessageTemplateCacheType{}
	for _, tc := range []struct {
		name    string
		set     func(int64)
		get     func(int64) bool
		ids     func() []int64
		reset   func()
		changed func(int64, int64) bool
	}{
		{
			name:    "alert",
			set:     func(id int64) { alert.Set(map[int64]*models.AlertRule{id: {Id: id}}, 1, 10) },
			get:     func(id int64) bool { return alert.Get(id) != nil },
			ids:     alert.GetRuleIds,
			reset:   alert.Reset,
			changed: alert.StatChanged,
		},
		{
			name:    "notify",
			set:     func(id int64) { notify.Set(map[int64]*models.NotifyRule{id: {ID: id}}, 1, 10) },
			get:     func(id int64) bool { return notify.Get(id) != nil },
			ids:     notify.GetRuleIds,
			reset:   notify.Reset,
			changed: notify.StatChanged,
		},
		{
			name:    "recording",
			set:     func(id int64) { record.Set(map[int64]*models.RecordingRule{id: {Id: id}}, 1, 10) },
			get:     func(id int64) bool { return record.Get(id) != nil },
			ids:     record.GetRuleIds,
			reset:   record.Reset,
			changed: record.StatChanged,
		},
		{
			name:    "pipeline",
			set:     func(id int64) { pipeline.Set(map[int64]*models.EventPipeline{id: {ID: id}}, 1, 10) },
			get:     func(id int64) bool { return pipeline.Get(id) != nil },
			ids:     pipeline.GetProcessorIds,
			reset:   pipeline.Reset,
			changed: pipeline.StatChanged,
		},
		{
			name:    "template",
			set:     func(id int64) { tpl.Set(map[int64]*models.MessageTemplate{id: {ID: id}}, 1, 10) },
			get:     func(id int64) bool { return tpl.Get(id) != nil },
			ids:     tpl.GetTemplateIds,
			reset:   tpl.Reset,
			changed: tpl.StatChanged,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.set(1)

			require.True(t, tc.get(1))
			require.False(t, tc.get(99))
			require.Equal(t, []int64{1}, tc.ids())
			require.False(t, tc.changed(1, 10))
			require.True(t, tc.changed(2, 10))
			require.True(t, tc.changed(1, 11))

			tc.set(2)

			require.False(t, tc.get(1))
			require.True(t, tc.get(2))

			tc.reset()

			require.Empty(t, tc.ids())
			require.False(t, tc.get(2))
			require.True(t, tc.changed(1, 10))
		})
	}
}

func TestUserCache_LookupsActivityAndReplacement(t *testing.T) {
	uc := &UserCacheType{}
	a := &models.User{Id: 1, Username: "alice", Maintainer: 1}
	b := &models.User{Id: 2, Username: "bob"}
	uc.Set(map[int64]*models.User{1: a, 2: b}, 2, 10, 3, 20)

	require.Same(t, a, uc.GetByUserId(1))
	require.Same(t, b, uc.GetByUsername("bob"))
	require.Nil(t, uc.GetByUsername("missing"))
	require.Nil(t, uc.GetByUserId(99))
	require.Equal(t, []*models.User{b, a}, uc.GetByUserIds([]int64{2, 99, 1, 2}))
	require.Empty(t, uc.GetByUserIds(nil))
	require.ElementsMatch(t, []*models.User{a, b}, uc.GetAllUsers())
	require.Equal(t, []*models.User{a}, uc.GetMaintainerUsers())

	for _, stat := range []struct {
		total, updated, configs, configsUpdated int64
		changed                                 bool
	}{
		{total: 2, updated: 10, configs: 3, configsUpdated: 20, changed: false},
		{total: 3, updated: 10, configs: 3, configsUpdated: 20, changed: true},
		{total: 2, updated: 11, configs: 3, configsUpdated: 20, changed: true},
		{total: 2, updated: 10, configs: 4, configsUpdated: 20, changed: true},
		{total: 2, updated: 10, configs: 3, configsUpdated: 21, changed: true},
	} {
		require.Equal(t, stat.changed, uc.StatChanged(stat.total, stat.updated, stat.configs, stat.configsUpdated))
	}
	uc.SetLastActiveTime(1, 123)
	uc.SetLastActiveTime(99, 123)

	require.Equal(t, int64(123), uc.GetByUserId(1).LastActiveTime)

	uc.Set(map[int64]*models.User{}, 0, 11, 3, 20)

	require.Nil(t, uc.GetByUserId(1))
	require.Empty(t, uc.GetMaintainerUsers())
}

func TestUserGroupCache_LookupsAndStatChanges(t *testing.T) {
	ug := &UserGroupCacheType{}
	g := &models.UserGroup{Id: 7, UserIds: []int64{1, 2}}
	ug.Set(map[int64]*models.UserGroup{7: g}, 1, 10)

	require.Same(t, g, ug.GetByUserGroupId(7))
	require.Nil(t, ug.GetByUserGroupId(8))
	require.Equal(t, []*models.UserGroup{g}, ug.GetByUserGroupIds([]int64{7, 8, 7}))
	require.Empty(t, ug.GetByUserGroupIds(nil))
	require.False(t, ug.StatChanged(1, 10))
	require.True(t, ug.StatChanged(2, 10))
	require.True(t, ug.StatChanged(1, 11))
}

func TestBusiGroupCache_LookupsAndStatChanges(t *testing.T) {
	bg := &BusiGroupCacheType{}
	group := &models.BusiGroup{Id: 7, Name: "ops"}
	bg.Set(map[int64]*models.BusiGroup{7: group}, 1, 10)

	require.Same(t, group, bg.GetByBusiGroupId(7))
	require.Nil(t, bg.GetByBusiGroupId(8))
	require.Equal(t, "ops", bg.GetNameByBusiGroupId(7))
	require.Empty(t, bg.GetNameByBusiGroupId(8))
	require.Equal(t, []string{"ops"}, bg.GetNamesByBusiGroupIds([]int64{8, 7}))
	require.False(t, bg.StatChanged(1, 10))
	require.True(t, bg.StatChanged(2, 10))
	require.True(t, bg.StatChanged(1, 11))
}

func TestAlertSubscribeCache_LookupsCopiesAndReset(t *testing.T) {
	c := &AlertSubscribeCacheType{}
	sub := &models.AlertSubscribe{Id: 1, Name: "ops"}
	c.Set(map[int64][]*models.AlertSubscribe{7: {sub}}, 1, 10)
	got, ok := c.Get(7)

	require.True(t, ok)
	require.Equal(t, []*models.AlertSubscribe{sub}, got)

	_, ok = c.Get(8)

	require.False(t, ok)
	require.Equal(t, []*models.AlertSubscribe{sub}, c.GetAll())
	require.Empty(t, c.GetStructs(8))

	copies := c.GetStructs(7)
	copies[0].Name = "changed"

	require.Equal(t, "ops", sub.Name)
	require.False(t, c.StatChanged(1, 10))
	require.True(t, c.StatChanged(2, 10))
	require.True(t, c.StatChanged(1, 11))

	c.Reset()

	require.Empty(t, c.GetAll())
	require.True(t, c.StatChanged(1, 10))
}

func TestAlertMuteCache_LookupsCopiesAndReset(t *testing.T) {
	m := &AlertMuteCacheType{}
	mute := &models.AlertMute{Id: 2, Note: "maintenance"}
	m.Set(map[int64][]*models.AlertMute{7: {mute}}, 1, 10)
	mutes, ok := m.Gets(7)

	require.True(t, ok)
	require.Equal(t, []*models.AlertMute{mute}, mutes)

	_, ok = m.Gets(8)

	require.False(t, ok)

	structs := m.GetAllStructs()

	require.Equal(t, "maintenance", structs[7][0].Note)

	structs[7][0].Note = "changed"

	require.Equal(t, "maintenance", mute.Note)
	require.False(t, m.StatChanged(1, 10))
	require.True(t, m.StatChanged(2, 10))
	require.True(t, m.StatChanged(1, 11))

	m.Reset()

	require.Empty(t, m.GetAllStructs())
	require.True(t, m.StatChanged(1, 10))
}

func TestDatasourceCache_ReplacementRebuildsCategoryIndexes(t *testing.T) {
	d := &DatasourceCacheType{}
	a := &models.Datasource{Id: 1, Name: "prom-a", PluginType: "prometheus"}
	b := &models.Datasource{Id: 2, Name: "logs", PluginType: "elasticsearch"}
	d.Set(map[int64]*models.Datasource{1: a, 2: b}, 2, 10)

	require.Same(t, a, d.GetById(1))
	require.Nil(t, d.GetById(99))
	require.Equal(t, []*models.Datasource{b}, d.GetByCate("elasticsearch"))
	require.Empty(t, d.GetByCate("unknown"))

	total, updated := d.GetStat()

	require.Equal(t, int64(2), total)
	require.Equal(t, int64(10), updated)
	require.False(t, d.StatChanged(2, 10))
	require.True(t, d.StatChanged(2, 11))
	require.True(t, d.StatChanged(3, 10))

	d.Set(map[int64]*models.Datasource{1: {Id: 1, Name: "new", PluginType: "mysql"}}, 1, 11)

	require.Nil(t, d.GetById(2))
	require.Empty(t, d.GetByCate("prometheus"))
	require.Empty(t, d.CateToNames["prometheus"])
	require.Equal(t, int64(1), d.CateToNames["mysql"]["new"])
}

func TestConfigCache_ReturnsCopiesAndReplacesValues(t *testing.T) {
	c := &ConfigCache{}
	c.Set(map[string]string{"region": "west"}, 1, 10)
	copy := c.Get()
	copy["region"] = "east"

	require.Equal(t, "west", c.Get()["region"])
	require.Equal(t, int64(10), c.GetLastUpdateTime())
	require.False(t, c.statChanged(1, 10))
	require.True(t, c.statChanged(2, 10))
	require.True(t, c.statChanged(1, 11))

	c.Set(map[string]string{}, 0, 11)

	require.Empty(t, c.Get())
}

func TestTaskTplCache_LookupsAndReplacement(t *testing.T) {
	task := &TaskTplCache{}
	tpl := &models.TaskTpl{Id: 1}
	task.Set(map[int64]*models.TaskTpl{1: tpl}, 1, 10)

	require.Same(t, tpl, task.Get(1))
	require.Nil(t, task.Get(2))
	require.False(t, task.StatChange(1, 10))
	require.True(t, task.StatChange(2, 10))
	require.True(t, task.StatChange(1, 11))

	task.Set(map[int64]*models.TaskTpl{}, 0, 11)

	require.Nil(t, task.Get(1))
}

func TestEsIndexPatternCache_LookupsAndReset(t *testing.T) {
	index := &EsIndexPatternCacheType{}
	pattern := &models.EsIndexPattern{Id: 1}
	index.Set(map[int64]*models.EsIndexPattern{1: pattern})
	got, ok := index.Get(1)

	require.True(t, ok)
	require.Same(t, pattern, got)

	_, ok = index.Get(2)

	require.False(t, ok)

	index.Reset()
	_, ok = index.Get(1)

	require.False(t, ok)
}

func TestIdentCountCache_IncrementAndFlush(t *testing.T) {
	counts := &IdentCountCacheType{idents: make(map[string]Item)}

	require.False(t, counts.Exists("host"))
	require.Zero(t, counts.Get("host"))

	counts.Set("host", 2, 100)
	counts.Increment("host", 3)
	counts.Increment("new", 1)

	require.True(t, counts.Exists("host"))
	require.Equal(t, 5, counts.Get("host"))

	flushed := counts.GetsAndFlush()

	require.Equal(t, 5, flushed["host"].Count)
	require.Equal(t, 1, flushed["new"].Count)
	require.False(t, counts.Exists("host"))

	counts.Set("host", 9, 200)

	require.Equal(t, 5, flushed["host"].Count)
}

func TestCvalCache_SiteConfiguration(t *testing.T) {
	c := &CvalCache{cvals: make(map[string]string)}
	c.Set([]*models.Configs{{
		Ckey: "site_info",
		Cval: `{"site_url":"https://n9e.test","print_access_log":true,"print_body_paths":["/events","/events","/rules"]}`,
	}}, 1, 10)

	require.Equal(t, "https://n9e.test", c.GetSiteInfo().SiteUrl)
	require.True(t, c.PrintAccessLog())
	require.Equal(t, map[string]struct{}{"/events": {}, "/rules": {}}, c.PrintBodyPaths())
	require.Empty(t, c.Get("missing"))
	require.Equal(t, int64(10), c.GetLastUpdateTime())
	require.False(t, c.statChanged(1, 10))
	require.True(t, c.statChanged(2, 10))
	require.True(t, c.statChanged(1, 11))
}

func TestCvalCache_InvalidSiteConfiguration(t *testing.T) {
	c := &CvalCache{cvals: make(map[string]string)}

	c.Set([]*models.Configs{{Ckey: "site_info", Cval: `{`}}, 1, 11)

	require.Empty(t, c.GetSiteInfo().SiteUrl)
}
