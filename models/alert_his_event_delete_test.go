package models_test

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newEventTestCtx(t *testing.T) *ctx.Context {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AlertHisEvent{}, &models.AlertCurEvent{}))
	return &ctx.Context{DB: db, IsCenter: true}
}

func hisEventIds(t *testing.T, c *ctx.Context) []int64 {
	var ids []int64
	require.NoError(t, c.DB.Model(&models.AlertHisEvent{}).Order("id asc").Pluck("id", &ids).Error)
	return ids
}

// 活跃告警对应的历史记录（cur.id == his.id）要跳过，其余照删；游标按候选推进。
func TestAlertHisEventBatchDeleteSkipsActive(t *testing.T) {
	c := newEventTestCtx(t)

	// 5 条历史记录，last_eval_time 都早于 cutoff；其中 id=2、4 仍是活跃告警
	for i := int64(1); i <= 5; i++ {
		require.NoError(t, c.DB.Create(&models.AlertHisEvent{Id: i, Hash: "h", Severity: 2, LastEvalTime: 100 + i}).Error)
	}
	for _, id := range []int64{2, 4} {
		require.NoError(t, c.DB.Create(&models.AlertCurEvent{Id: id, Hash: "h", Severity: 2}).Error)
	}

	activeIds, err := models.AlertCurEventIds(c)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{2, 4}, activeIds)

	active := map[int64]struct{}{}
	for _, id := range activeIds {
		active[id] = struct{}{}
	}

	fetched, deleted, maxId, err := models.AlertHisEventBatchDelete(c, 1000, nil, 0, 100, active)
	require.NoError(t, err)
	assert.Equal(t, 5, fetched)
	assert.Equal(t, int64(3), deleted)
	assert.Equal(t, int64(5), maxId)
	assert.Equal(t, []int64{2, 4}, hisEventIds(t, c), "活跃告警对应的历史记录应保留")
}

// 模拟路由层循环：跳过的活跃记录数超过批大小时，游标保证循环仍能终止。
func TestAlertHisEventBatchDeleteCursorTerminates(t *testing.T) {
	c := newEventTestCtx(t)

	// 7 条候选全部对应活跃告警，批大小 3：不推进游标会死循环
	active := map[int64]struct{}{}
	for i := int64(1); i <= 7; i++ {
		require.NoError(t, c.DB.Create(&models.AlertHisEvent{Id: i, Hash: "h", Severity: 2, LastEvalTime: 100}).Error)
		active[i] = struct{}{}
	}

	limit := 3
	var minId int64
	rounds := 0
	for {
		rounds++
		require.LessOrEqual(t, rounds, 10, "循环未终止")
		fetched, deleted, maxId, err := models.AlertHisEventBatchDelete(c, 1000, nil, minId, limit, active)
		require.NoError(t, err)
		assert.Equal(t, int64(0), deleted)
		if fetched < limit {
			break
		}
		minId = maxId
	}
	assert.Equal(t, 3, rounds) // 3+3+1
	assert.Len(t, hisEventIds(t, c), 7, "全部为活跃告警，一条都不该删")
}

// severity 过滤与 last_eval_time 边界维持原语义。
func TestAlertHisEventBatchDeleteFilters(t *testing.T) {
	c := newEventTestCtx(t)

	require.NoError(t, c.DB.Create(&models.AlertHisEvent{Id: 1, Hash: "h", Severity: 1, LastEvalTime: 100}).Error)
	require.NoError(t, c.DB.Create(&models.AlertHisEvent{Id: 2, Hash: "h", Severity: 2, LastEvalTime: 100}).Error)
	require.NoError(t, c.DB.Create(&models.AlertHisEvent{Id: 3, Hash: "h", Severity: 1, LastEvalTime: 2000}).Error)

	fetched, deleted, _, err := models.AlertHisEventBatchDelete(c, 1000, []int{1}, 0, 100, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, fetched)
	assert.Equal(t, int64(1), deleted)
	assert.Equal(t, []int64{2, 3}, hisEventIds(t, c), "severity 不匹配和时间未到的记录应保留")
}
