package models

import (
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/secu"
	"github.com/stretchr/testify/require"
)

func TestR8dRuleKinds(t *testing.T) {
	for _, tc := range []struct {
		prod, cate                             string
		prom, loki, host, inner, td, click, es bool
		kind                                   string
	}{
		{METRIC, PROMETHEUS, true, false, false, false, false, false, false, PROMETHEUS},
		{LOKI, "", false, true, false, true, false, false, false, LOKI},
		{METRIC, LOKI, false, true, false, true, false, false, false, LOKI},
		{HOST, HOST, false, false, true, false, false, false, false, HOST},
		{METRIC, TDENGINE, false, false, false, true, true, false, false, TDENGINE},
		{LOG, CLICKHOUSE, false, false, false, true, false, true, false, CLICKHOUSE},
		{LOG, ELASTICSEARCH, false, false, false, true, false, false, true, ELASTICSEARCH},
		{METRIC, MYSQL, false, false, false, true, false, false, false, MYSQL},
	} {
		t.Run(tc.prod+tc.cate, func(t *testing.T) {
			ar := &AlertRule{Prod: tc.prod, Cate: tc.cate}
			require.Equal(t, tc.prom, ar.IsPrometheusRule())
			require.Equal(t, tc.loki, ar.IsLokiRule())
			require.Equal(t, tc.host, ar.IsHostRule())
			require.Equal(t, tc.inner, ar.IsInnerRule())
			require.Equal(t, tc.td, ar.IsTdengineRule())
			require.Equal(t, tc.click, ar.IsClickHouseRule())
			require.Equal(t, tc.es, ar.IsElasticSearch())
			require.Equal(t, tc.kind, ar.GetRuleType())
		})
	}
}
func TestR8dEventCountsByRule(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	require.NoError(t, c.DB.Create(&[]AlertCurEvent{{Hash: "a", RuleId: 7, TriggerTime: 10}, {Hash: "b", RuleId: 7, TriggerTime: 20}, {Hash: "c", RuleId: 8, TriggerTime: 30}}).Error)
	require.Equal(t, map[int64]int64{7: 2}, AlertCurEventCountByRuleId(c, []int64{7}, 10, 20))
	require.Empty(t, AlertCurEventCountByRuleId(c, nil, 50, 60))
	require.NoError(t, c.DB.Migrator().DropTable(&AlertCurEvent{}))
	require.Nil(t, AlertCurEventCountByRuleId(c, nil, 0, 100))
}
func TestR8dUserListFiltersAndActivity(t *testing.T) {
	c := pipelineContext(t, &User{}, &UserGroupMember{}, &UserGroup{}, &BusiGroup{}, &BusiGroupMember{})
	require.NoError(t, c.DB.Create(&[]User{{Id: 1, Username: "alice", Phone: "123", Email: "a@test", LastActiveTime: 50}, {Id: 2, Username: "bob", LastActiveTime: 90}}).Error)
	users, err := UserGets(c, "", 10, 0, 1, 100, "username", true, []string{"alice", "bob"}, []string{"123"}, []string{"a@test"}, nil)
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, "alice", users[0].Username)
	require.NoError(t, UpdateUserLastActiveTime(c, 1, 75))
	user, err := UserGetById(c, 1)
	require.NoError(t, err)
	require.Equal(t, int64(75), user.LastActiveTime)
	total, err := UserTotal(c, "", 0, 0, nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	for _, input := range []string{" ", string(make([]byte, 65))} {
		require.Equal(t, "username", validateOrderField(input, "username"))
	}
}
func TestR8dDatasourceDecryptFailures(t *testing.T) {
	private, public, err := secu.GenerateRsaKeyPair("pass")
	require.NoError(t, err)
	old := rsaConfig
	t.Cleanup(func() { rsaConfig = old })
	SetRsaConfig(&RsaConfig{OpenRSA: true, PrivateKeyBytes: private, RSAPassWord: "pass"})
	require.Same(t, rsaConfig, GetRsaConfig())
	for _, field := range []string{"settings", "auth"} {
		for _, plain := range []string{"bad ciphertext", "invalid JSON"} {
			t.Run(field+plain, func(t *testing.T) {
				value := plain
				if plain == "invalid JSON" {
					value, err = secu.EncryptValue(plain, public)
					require.NoError(t, err)
				}
				ds := Datasource{}
				if field == "settings" {
					ds.SettingsEncoded = value
				} else {
					ds.AuthEncoded = value
				}
				require.Error(t, ds.Decrypt())
			})
		}
	}
	for _, ds := range []Datasource{{Settings: "secret"}, {Auth: "secret"}} {
		require.Error(t, ds.Encrypt(true, []byte("invalid key")))
	}
}
func TestR8dMuteListFilters(t *testing.T) {
	c := pipelineContext(t, &AlertMute{})
	now := time.Now().Unix()
	require.NoError(t, c.DB.Create(&[]AlertMute{
		{Id: 1, GroupId: 7, Prod: METRIC, Cause: "deploy api", MuteTimeType: TimeRange, Etime: now + 3600},
		{Id: 2, GroupId: 7, Prod: METRIC, Cause: "deploy api", MuteTimeType: TimeRange, Etime: now - 3600},
		{Id: 3, GroupId: 8, Disabled: 1, Prod: LOG, Cause: "other", MuteTimeType: Periodic},
	}).Error)
	for _, tc := range []struct {
		bg                int64
		disabled, expired int
		query             string
		prod              []string
		ids               []int64
	}{
		{7, 0, 0, "deploy api", []string{METRIC}, []int64{1}},
		{7, 0, 1, "deploy", []string{METRIC}, []int64{2}},
		{8, 1, -1, "", []string{LOG}, []int64{3}},
	} {
		rows, err := AlertMuteGets(c, tc.prod, tc.bg, tc.disabled, tc.expired, tc.query)
		require.NoError(t, err)
		var ids []int64
		for _, r := range rows {
			ids = append(ids, r.Id)
		}
		require.ElementsMatch(t, tc.ids, ids)
	}
}
