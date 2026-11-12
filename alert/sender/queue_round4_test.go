package sender

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestSafeList_PopsEventsInInsertionOrderAndSkipsForeignValues(t *testing.T) {
	queue := NewSafeList()
	first, second := &models.AlertCurEvent{Id: 1}, &models.AlertCurEvent{Id: 2}
	queue.PushFront(first)
	queue.PushFrontBatch([]interface{}{"foreign value", second})

	batch := queue.PopBack(2)

	require.Equal(t, []*models.AlertCurEvent{first}, batch)
	require.Equal(t, 1, queue.Len())
	require.Equal(t, []*models.AlertCurEvent{second}, queue.PopBack(10))
	require.Empty(t, queue.PopBack(10))
}

func TestSafeList_RemoveAllEmptiesPendingEvents(t *testing.T) {
	queue := NewSafeList()
	queue.PushFrontBatch([]interface{}{&models.AlertCurEvent{Id: 1}, &models.AlertCurEvent{Id: 2}})

	queue.RemoveAll()

	require.Zero(t, queue.Len())
	require.Empty(t, queue.PopBack(1))
}

func TestSafeListLimited_RejectsWritesWhenFullAndReusesCapacity(t *testing.T) {
	queue := NewSafeListLimited(2)
	first, second, third := &models.AlertCurEvent{Id: 1}, &models.AlertCurEvent{Id: 2}, &models.AlertCurEvent{Id: 3}
	require.True(t, queue.PushFrontBatch([]interface{}{first, second}))

	require.False(t, queue.PushFront(third))
	require.False(t, queue.PushFrontBatch([]interface{}{third}))
	require.Equal(t, 2, queue.Len())
	require.Equal(t, []*models.AlertCurEvent{first}, queue.PopBack(1))
	require.True(t, queue.PushFront(third))
	require.Equal(t, []*models.AlertCurEvent{second, third}, queue.PopBack(10))
	queue.RemoveAll()
	require.Zero(t, queue.Len())
}
