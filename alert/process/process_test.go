package process

import (
	"testing"

	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/models"
)

// 回归：队列里必须是事件快照，不能是 p.fires 持有的活对象。
//
// 历史 bug：pushEventToQueue 把同一个指针同时存进 p.fires 和事件队列，
// RecoverSingle 又从 p.fires 取出这个指针原地改成恢复事件后二次入队。
// 队列一旦积压，还没被消费的那条触发事件会被就地改写成恢复事件，
// 消费出来两条都是 recovered —— alert_his_event 落两条恢复、零条触发。
func TestPushEventToQueueSnapshotsEvent(t *testing.T) {
	queue.EventQueue.RemoveAll()
	defer queue.EventQueue.RemoveAll()

	p := &Processor{
		rule:                 &models.AlertRule{Id: 1},
		fires:                NewAlertCurEventMap(nil),
		pendings:             NewAlertCurEventMap(nil),
		pendingsUseByRecover: NewAlertCurEventMap(nil),
	}

	fireEvent := &models.AlertCurEvent{
		Hash:         "hash-1",
		RuleId:       1,
		LastEvalTime: 100,
		TagsJSON:     []string{"a=1"},
		TagsMap:      map[string]string{"a": "1"},
	}

	p.pushEventToQueue(fireEvent)

	fired, has := p.fires.Get("hash-1")
	if !has {
		t.Fatal("fire 事件应存入 p.fires")
	}
	if fired != fireEvent {
		t.Fatal("p.fires 应持有活对象本身，重复通知逻辑依赖它")
	}

	// 模拟 RecoverSingle：从 p.fires 取出同一个指针，原地改成恢复事件后二次入队
	p.fires.Delete("hash-1")
	fired.IsRecovered = true
	fired.LastEvalTime = 200
	fired.TagsMap["a"] = "2"
	p.pushEventToQueue(fired)

	// PushFront + PopBackBy 是 FIFO，先出触发、后出恢复
	items := queue.EventQueue.PopBackBy(10)
	if len(items) != 2 {
		t.Fatalf("队列应有 2 条事件，实际 %d 条", len(items))
	}

	queuedFire := items[0].(*models.AlertCurEvent)
	if queuedFire == fireEvent {
		t.Fatal("队列里不能放活对象，否则会被 recover 就地改写")
	}
	if queuedFire.IsRecovered {
		t.Fatal("队列中的触发事件被改写成了恢复事件")
	}
	if queuedFire.LastEvalTime != 100 {
		t.Fatalf("触发事件的 LastEvalTime 被改写：want 100, got %d", queuedFire.LastEvalTime)
	}
	if queuedFire.TagsMap["a"] != "1" {
		t.Fatalf("TagsMap 未深拷贝，被改写为 %q", queuedFire.TagsMap["a"])
	}

	queuedRecover := items[1].(*models.AlertCurEvent)
	if !queuedRecover.IsRecovered {
		t.Fatal("第二条应是恢复事件")
	}
	if queuedRecover.LastEvalTime != 200 {
		t.Fatalf("恢复事件的 LastEvalTime：want 200, got %d", queuedRecover.LastEvalTime)
	}
}

// 恢复事件不应写入 p.fires，且不应推进 LastSentTime。
func TestPushEventToQueueRecoveredNotTracked(t *testing.T) {
	queue.EventQueue.RemoveAll()
	defer queue.EventQueue.RemoveAll()

	p := &Processor{
		rule:                 &models.AlertRule{Id: 1},
		fires:                NewAlertCurEventMap(nil),
		pendings:             NewAlertCurEventMap(nil),
		pendingsUseByRecover: NewAlertCurEventMap(nil),
	}

	recoverEvent := &models.AlertCurEvent{
		Hash:         "hash-2",
		RuleId:       1,
		IsRecovered:  true,
		LastEvalTime: 300,
		LastSentTime: 100,
	}

	p.pushEventToQueue(recoverEvent)

	if _, has := p.fires.Get("hash-2"); has {
		t.Fatal("恢复事件不应写入 p.fires")
	}
	if recoverEvent.LastSentTime != 100 {
		t.Fatalf("恢复事件不应推进 LastSentTime，got %d", recoverEvent.LastSentTime)
	}

	items := queue.EventQueue.PopBackBy(10)
	if len(items) != 1 {
		t.Fatalf("队列应有 1 条事件，实际 %d 条", len(items))
	}
	if items[0].(*models.AlertCurEvent) == recoverEvent {
		t.Fatal("队列里不能放活对象")
	}
}
