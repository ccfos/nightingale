package models

import (
	"context"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAlertMuteTestCtx(t *testing.T) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AlertMute{}))
	return ctx.NewContext(context.Background(), db, true)
}

func insertMute(t *testing.T, c *ctx.Context, m *AlertMute) {
	t.Helper()
	require.NoError(t, DB(c).Create(m).Error)
}

func TestAlertMuteBatchDelete(t *testing.T) {
	c := newAlertMuteTestCtx(t)
	now := time.Now().Unix()
	threshold := now - 30*24*3600 // 30 days ago

	// expired before threshold + group 1 -> SHOULD delete
	insertMute(t, c, &AlertMute{GroupId: 1, MuteTimeType: TimeRange, Etime: threshold - 3600, CreateAt: threshold - 7200})
	// expired before threshold + group 2 -> SHOULD delete (when no group filter)
	insertMute(t, c, &AlertMute{GroupId: 2, MuteTimeType: TimeRange, Etime: threshold - 3600, CreateAt: threshold - 7200})
	// permanent mute (etime=0) -> SHOULD NOT delete
	insertMute(t, c, &AlertMute{GroupId: 1, MuteTimeType: TimeRange, Etime: 0, CreateAt: threshold - 7200})
	// not yet expired -> SHOULD NOT delete
	insertMute(t, c, &AlertMute{GroupId: 1, MuteTimeType: TimeRange, Etime: now + 3600, CreateAt: threshold - 7200})
	// expired after threshold -> SHOULD NOT delete
	insertMute(t, c, &AlertMute{GroupId: 1, MuteTimeType: TimeRange, Etime: now - 3600, CreateAt: threshold - 7200})
	// periodic mute (etime/create_at would otherwise qualify) -> SHOULD NOT delete
	insertMute(t, c, &AlertMute{GroupId: 1, MuteTimeType: Periodic, Etime: threshold - 3600, CreateAt: threshold - 7200})

	// Restrict to group 1: only the first row should match.
	n, err := AlertMuteBatchDelete(c, threshold, []int64{1}, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// Verify group 2's expired+old row is still there.
	var remaining []AlertMute
	require.NoError(t, DB(c).Find(&remaining).Error)
	assert.Equal(t, 5, len(remaining))

	// Now sweep with no group filter: should delete the group 2 expired+old row.
	n, err = AlertMuteBatchDelete(c, threshold, nil, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// Remaining should be: permanent, not-yet-expired, expired-after-threshold,
	// periodic (4 rows).
	require.NoError(t, DB(c).Find(&remaining).Error)
	assert.Equal(t, 4, len(remaining))
	for _, m := range remaining {
		switch {
		case m.MuteTimeType == Periodic:
		case m.Etime == 0:
		case m.Etime >= threshold:
		default:
			t.Errorf("unexpected row not pruned: %+v", m)
		}
	}
}

func TestAlertMuteBatchDelete_NoMatch(t *testing.T) {
	c := newAlertMuteTestCtx(t)
	now := time.Now().Unix()
	threshold := now - 30*24*3600

	// permanent mute
	insertMute(t, c, &AlertMute{GroupId: 1, Etime: 0, CreateAt: threshold - 100})
	// not expired yet
	insertMute(t, c, &AlertMute{GroupId: 1, Etime: now + 3600, CreateAt: threshold - 100})

	n, err := AlertMuteBatchDelete(c, threshold, nil, 1000)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	var remaining []AlertMute
	require.NoError(t, DB(c).Find(&remaining).Error)
	assert.Equal(t, 2, len(remaining))
}

// Regression: a mute whose btime is still in the future must be returned by
// AlertMuteGetsAll. The engine cache only re-syncs when count/max(update_at)
// changes, and reaching btime changes neither, so excluding such a mute in SQL
// meant it never entered the cache once its window opened.
func TestAlertMuteGetsAll_IncludesFutureBtime(t *testing.T) {
	c := newAlertMuteTestCtx(t)
	now := time.Now().Unix()

	// not yet active (btime 20 minutes in the future) -> MUST be returned
	insertMute(t, c, &AlertMute{Id: 1, GroupId: 1, MuteTimeType: TimeRange, Btime: now + 1200, Etime: now + 3600})
	// currently active -> returned
	insertMute(t, c, &AlertMute{Id: 2, GroupId: 1, MuteTimeType: TimeRange, Btime: now - 600, Etime: now + 600})
	// already expired -> NOT returned
	insertMute(t, c, &AlertMute{Id: 3, GroupId: 1, MuteTimeType: TimeRange, Btime: now - 7200, Etime: now - 3600})
	// disabled -> NOT returned
	insertMute(t, c, &AlertMute{Id: 4, GroupId: 1, MuteTimeType: TimeRange, Btime: now - 600, Etime: now + 600, Disabled: 1})
	// periodic -> returned regardless of btime/etime
	insertMute(t, c, &AlertMute{Id: 5, GroupId: 1, MuteTimeType: Periodic, PeriodicMutes: "[]"})

	lst, err := AlertMuteGetsAll(c)
	require.NoError(t, err)

	got := make([]int64, 0, len(lst))
	for _, m := range lst {
		got = append(got, m.Id)
	}
	assert.ElementsMatch(t, []int64{1, 2, 5}, got)
}

func TestPeriodicMuteMatch(t *testing.T) {
	at := func(s string) time.Time {
		tm, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
		require.NoError(t, err)
		return tm
	}
	monthly := func(stime, etime string, days ...int) PeriodicMute {
		return PeriodicMute{Cycle: PeriodicCycleMonth, EnableStime: stime, EnableEtime: etime, EnableDaysOfMonth: days}
	}

	tests := []struct {
		name string
		p    PeriodicMute
		at   string
		want bool
	}{
		{"weekly legacy empty cycle", PeriodicMute{EnableDaysOfWeek: "1 2", EnableStime: "00:00", EnableEtime: "06:00"}, "2026-11-23 03:00", true}, // Monday
		{"weekly other day", PeriodicMute{Cycle: PeriodicCycleWeek, EnableDaysOfWeek: "1 2", EnableStime: "00:00", EnableEtime: "06:00"}, "2026-11-25 03:00", false},
		{"monthly day in window", monthly("00:00", "06:00", 1, 15), "2026-12-15 05:59", true},
		{"monthly window end is exclusive", monthly("00:00", "06:00", 1, 15), "2026-12-15 06:00", false},
		{"monthly other day", monthly("00:00", "06:00", 1, 15), "2026-12-14 03:00", false},
		{"monthly day 1 does not match 11", monthly("00:00", "06:00", 1), "2026-12-11 03:00", false},
		{"monthly day 5 does not match 15", monthly("00:00", "06:00", 5), "2026-12-15 03:00", false},
		{"monthly all day", monthly("00:00", "23:59", 31), "2026-12-31 23:59", true},
		{"monthly end 00:00 covers 23:59", monthly("23:00", "00:00", LastDayOfMonth), "2026-11-30 23:59", true},
		{"monthly end 00:00 stays on the day", monthly("23:00", "00:00", LastDayOfMonth), "2026-11-30 22:59", false},
		{"monthly end 00:00 does not reach next day", monthly("23:00", "00:00", LastDayOfMonth), "2026-12-01 00:30", false},
		{"monthly 31 skipped in 30-day month", monthly("00:00", "23:59", 31), "2026-11-30 12:00", false},
		{"last day of 30-day month", monthly("00:00", "23:59", LastDayOfMonth), "2026-11-30 12:00", true},
		{"last day of february", monthly("00:00", "23:59", LastDayOfMonth), "2027-02-28 12:00", true},
		{"last day of leap february", monthly("00:00", "23:59", LastDayOfMonth), "2028-02-28 12:00", false},
		{"leap february 29", monthly("00:00", "23:59", LastDayOfMonth), "2028-02-29 12:00", true},
		{"monthly ignores days of week", PeriodicMute{Cycle: PeriodicCycleMonth, EnableDaysOfWeek: "0 1 2 3 4 5 6", EnableStime: "00:00", EnableEtime: "23:59"}, "2026-12-15 12:00", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.p.Match(at(tt.at)))
		})
	}
}

func TestPeriodicMuteVerify(t *testing.T) {
	tests := []struct {
		name    string
		p       PeriodicMute
		wantErr string
	}{
		{"weekly is not checked", PeriodicMute{EnableDaysOfWeek: "1", EnableStime: "22:00", EnableEtime: "02:00"}, ""},
		{"monthly ok", PeriodicMute{Cycle: PeriodicCycleMonth, EnableDaysOfMonth: []int{1, 31, LastDayOfMonth}, EnableStime: "00:00", EnableEtime: "06:00"}, ""},
		{"monthly all day", PeriodicMute{Cycle: PeriodicCycleMonth, EnableDaysOfMonth: []int{1}, EnableStime: "00:00", EnableEtime: "00:00"}, ""},
		{"unknown cycle", PeriodicMute{Cycle: "year"}, "invalid periodic mute cycle"},
		{"monthly without days", PeriodicMute{Cycle: PeriodicCycleMonth, EnableStime: "00:00", EnableEtime: "06:00"}, "monthly periodic mute requires at least one day of month"},
		{"monthly day out of range", PeriodicMute{Cycle: PeriodicCycleMonth, EnableDaysOfMonth: []int{32}, EnableStime: "00:00", EnableEtime: "06:00"}, "day of month must be between 1 and 31, or -1 for the last day"},
		{"monthly day zero", PeriodicMute{Cycle: PeriodicCycleMonth, EnableDaysOfMonth: []int{0}, EnableStime: "00:00", EnableEtime: "06:00"}, "day of month must be between 1 and 31, or -1 for the last day"},
		{"monthly bad time", PeriodicMute{Cycle: PeriodicCycleMonth, EnableDaysOfMonth: []int{1}, EnableStime: "0:00", EnableEtime: "06:00"}, "invalid periodic mute time, expected HH:MM"},
		{"monthly until midnight", PeriodicMute{Cycle: PeriodicCycleMonth, EnableDaysOfMonth: []int{LastDayOfMonth}, EnableStime: "23:00", EnableEtime: "00:00"}, ""},
		{"monthly cross midnight", PeriodicMute{Cycle: PeriodicCycleMonth, EnableDaysOfMonth: []int{1}, EnableStime: "23:00", EnableEtime: "03:00"}, "monthly periodic mute cannot span midnight, split it into two rows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Verify()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}

// Monthly rows are stored without days of week so nodes that only know weekly
// mutes skip them instead of muting; weekly rows read back with an empty
// days-of-month array rather than null.
func TestAlertMuteMonthlyRoundTrip(t *testing.T) {
	c := newAlertMuteTestCtx(t)
	now := time.Now().Unix()
	m := &AlertMute{
		GroupId: 1, Cause: "billing", Btime: now, Etime: now + 3600, MuteTimeType: Periodic,
		PeriodicMutesJson: []PeriodicMute{
			{Cycle: PeriodicCycleMonth, EnableDaysOfWeek: "0 1 2 3 4 5 6", EnableDaysOfMonth: []int{1, LastDayOfMonth}, EnableStime: "00:00", EnableEtime: "06:00"},
			{EnableDaysOfWeek: "0 6", EnableStime: "00:00", EnableEtime: "23:59"},
		},
	}
	require.NoError(t, m.Add(c))

	got, err := AlertMuteGetById(c, m.Id)
	require.NoError(t, err)
	require.Len(t, got.PeriodicMutesJson, 2)
	assert.Equal(t, PeriodicCycleMonth, got.PeriodicMutesJson[0].Cycle)
	assert.Equal(t, "", got.PeriodicMutesJson[0].EnableDaysOfWeek)
	assert.Equal(t, []int{1, LastDayOfMonth}, got.PeriodicMutesJson[0].EnableDaysOfMonth)
	assert.Equal(t, []int{}, got.PeriodicMutesJson[1].EnableDaysOfMonth)

	// A row written by an older version has no cycle and no days of month.
	insertMute(t, c, &AlertMute{Id: 100, GroupId: 1, MuteTimeType: Periodic, DatasourceIds: "[]",
		PeriodicMutes: `[{"enable_stime":"00:00","enable_etime":"06:00","enable_days_of_week":"1"}]`})
	legacy, err := AlertMuteGetById(c, 100)
	require.NoError(t, err)
	assert.Equal(t, []int{}, legacy.PeriodicMutesJson[0].EnableDaysOfMonth)
	assert.True(t, legacy.IsWithinPeriodicMute(time.Date(2026, 11, 23, 3, 0, 0, 0, time.Local).Unix()))
}
