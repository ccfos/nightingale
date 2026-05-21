package sender

import (
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/assert"
)

func TestSafePriorityQueue_ConcurrentPushPop(t *testing.T) {
	spq := NewSafeEventQueue(100000)

	var wg sync.WaitGroup
	numGoroutines := 100
	numEvents := 1000

	// 并发 Push
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numEvents; j++ {
				event := &models.AlertCurEvent{
					Severity:    goroutineID%3 + 1,
					TriggerTime: time.Now().UnixNano(),
				}
				spq.Push(event)
			}
		}(i)
	}
	wg.Wait()

	// 检查队列长度是否正确
	expectedLen := numGoroutines * numEvents
	assert.Equal(t, expectedLen, spq.Len(), "Queue length mismatch after concurrent pushes")

	// 并发 Pop
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for {
				event := spq.Pop()
				if event == nil {
					return
				}
			}
		}()
	}
	wg.Wait()

	// 最终队列应该为空
	assert.Equal(t, 0, spq.Len(), "Queue should be empty after concurrent pops")
}

func TestSafePriorityQueue_ConcurrentPopMax(t *testing.T) {
	spq := NewSafeEventQueue(100000)

	// 添加初始数据
	for i := 0; i < 1000; i++ {
		spq.Push(&models.AlertCurEvent{
			Severity:    i%3 + 1,
			TriggerTime: time.Now().UnixNano(),
		})
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	popMax := 100

	// 并发 PopN
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			events := spq.PopN(popMax)
			assert.LessOrEqual(t, len(events), popMax, "PopN exceeded maximum")
		}()
	}
	wg.Wait()

	// 检查队列长度是否正确
	expectedRemaining := 1000 - (numGoroutines * popMax)
	if expectedRemaining < 0 {
		expectedRemaining = 0
	}
	assert.Equal(t, expectedRemaining, spq.Len(), "Queue length mismatch after concurrent PopN")
}

func TestSafePriorityQueue_ConcurrentPushPopWithDifferentSeverities(t *testing.T) {
	spq := NewSafeEventQueue(100000)

	var wg sync.WaitGroup
	numGoroutines := 50
	numEvents := 500

	// 并发 Push 不同优先级的事件
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numEvents; j++ {
				event := &models.AlertCurEvent{
					Severity:    goroutineID%3 + 1, // 模拟不同的 Severity
					TriggerTime: time.Now().UnixNano(),
				}
				spq.Push(event)
			}
		}(i)
	}
	wg.Wait()

	// 检查队列长度是否正确
	expectedLen := numGoroutines * numEvents
	assert.Equal(t, expectedLen, spq.Len(), "Queue length mismatch after concurrent pushes")

	// 检查事件的顺序是否按照优先级排列
	var lastEvent *models.AlertCurEvent
	for spq.Len() > 0 {
		event := spq.Pop()
		if lastEvent != nil {
			assert.LessOrEqual(t, lastEvent.Severity, event.Severity, "Events are not in correct priority order")
		}
		lastEvent = event
	}
}

func TestSafePriorityQueue_ExceedMaxSize(t *testing.T) {
	spq := NewSafeEventQueue(5)

	for i := 0; i < spq.maxSize; i++ {
		ok := spq.Push(&models.AlertCurEvent{
			Severity:    i%3 + 1,
			TriggerTime: int64(i),
		})
		assert.True(t, ok)
	}

	ok := spq.Push(&models.AlertCurEvent{
		Severity:    High,
		TriggerTime: int64(spq.maxSize),
	})
	assert.False(t, ok)
	assert.Equal(t, spq.maxSize, spq.Len())

	// 验证队列中剩余事件的内容
	for i := 0; i < spq.maxSize; i++ {
		event := spq.Pop()
		assert.NotNil(t, event)
		assert.GreaterOrEqual(t, event.Severity, High)
		assert.LessOrEqual(t, event.Severity, Low)
	}
	assert.Equal(t, 0, spq.Len())
}
