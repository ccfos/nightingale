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

	// expired + created long ago + group 1 -> SHOULD delete
	insertMute(t, c, &AlertMute{GroupId: 1, Etime: now - 3600, CreateAt: threshold - 100})
	// expired + created long ago + group 2 -> SHOULD delete (when no group filter)
	insertMute(t, c, &AlertMute{GroupId: 2, Etime: now - 3600, CreateAt: threshold - 100})
	// permanent mute (etime=0) -> SHOULD NOT delete
	insertMute(t, c, &AlertMute{GroupId: 1, Etime: 0, CreateAt: threshold - 100})
	// not yet expired -> SHOULD NOT delete
	insertMute(t, c, &AlertMute{GroupId: 1, Etime: now + 3600, CreateAt: threshold - 100})
	// expired but created recently -> SHOULD NOT delete
	insertMute(t, c, &AlertMute{GroupId: 1, Etime: now - 3600, CreateAt: now - 10})

	// Restrict to group 1: only the first row should match.
	n, err := AlertMuteBatchDelete(c, threshold, []int64{1}, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// Verify group 2's expired+old row is still there.
	var remaining []AlertMute
	require.NoError(t, DB(c).Find(&remaining).Error)
	assert.Equal(t, 4, len(remaining))

	// Now sweep with no group filter: should delete the group 2 expired+old row.
	n, err = AlertMuteBatchDelete(c, threshold, nil, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// Remaining should be: permanent, not-yet-expired, created-recently (3 rows).
	require.NoError(t, DB(c).Find(&remaining).Error)
	assert.Equal(t, 3, len(remaining))
	for _, m := range remaining {
		switch {
		case m.Etime == 0:
		case m.Etime > now:
		case m.CreateAt >= threshold:
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
