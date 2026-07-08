package mute

import (
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
)

// 只配了生效星期却没配起止时间时三数组长度不一致，修复前会 panic，验证兜底后不再 panic 且规则被 mute。
func TestTimeSpanMuteStrategy_MismatchedLengthNoPanic(t *testing.T) {
	rule := &models.AlertRule{
		EnableStime:      "",
		EnableEtime:      "",
		EnableDaysOfWeek: "0 1 2 3 4 5 6",
	}
	event := &models.AlertCurEvent{TriggerTime: time.Now().Unix()}
	if !TimeSpanMuteStrategy(rule, event) {
		t.Fatalf("expected muted=true for incomplete time span config, got false")
	}
}

// 正常全天全星期配置不应被 mute，验证兜底不影响正常判断。
func TestTimeSpanMuteStrategy_FullDayNotMuted(t *testing.T) {
	rule := &models.AlertRule{
		EnableStime:      "00:00",
		EnableEtime:      "23:59",
		EnableDaysOfWeek: "0 1 2 3 4 5 6",
	}
	event := &models.AlertCurEvent{TriggerTime: time.Now().Unix()}
	if TimeSpanMuteStrategy(rule, event) {
		t.Fatalf("expected muted=false for full-day config, got true")
	}
}

// 命中「只屏蔽通知」规则时 EventMuteStrategy 应回传 MuteTypeNotifyOnly（上层据此保留事件、仅拦通知），
// 命中「屏蔽事件与通知」规则则回传 MuteTypeAll，未命中同样回传 MuteTypeAll。
func TestEventMuteStrategy_MuteType(t *testing.T) {
	now := time.Now().Unix()
	newMute := func(muteType int) *models.AlertMute {
		return &models.AlertMute{
			GroupId:      1,
			MuteTimeType: models.TimeRange,
			Btime:        now - 100,
			Etime:        now + 100,
			MuteType:     muteType,
		}
	}
	event := &models.AlertCurEvent{GroupId: 1, TriggerTime: now, TagsMap: map[string]string{}}
	cache := &memsto.AlertMuteCacheType{}

	cache.Set(map[int64][]*models.AlertMute{1: {newMute(models.MuteTypeNotifyOnly)}}, 1, now)
	if hit, _, muteType := EventMuteStrategy(event, cache); !hit || muteType != models.MuteTypeNotifyOnly {
		t.Fatalf("notify-only mute: got hit=%v muteType=%d, want hit=true muteType=%d", hit, muteType, models.MuteTypeNotifyOnly)
	}

	cache.Set(map[int64][]*models.AlertMute{1: {newMute(models.MuteTypeAll)}}, 1, now)
	if hit, _, muteType := EventMuteStrategy(event, cache); !hit || muteType != models.MuteTypeAll {
		t.Fatalf("mute-all: got hit=%v muteType=%d, want hit=true muteType=%d", hit, muteType, models.MuteTypeAll)
	}

	// 时间窗口外：不命中，屏蔽方式回落到 MuteTypeAll
	event.TriggerTime = now + 100000
	if hit, _, muteType := EventMuteStrategy(event, cache); hit || muteType != models.MuteTypeAll {
		t.Fatalf("no match: got hit=%v muteType=%d, want hit=false muteType=%d", hit, muteType, models.MuteTypeAll)
	}
}

// 事件同时命中「屏蔽事件与通知」与「只屏蔽通知」两条规则时，无论规则在缓存中的先后，
// 都应以更强的 MuteTypeAll 生效，并返回该完全屏蔽规则的 id。
func TestEventMuteStrategy_MuteAllWinsOverNotifyOnly(t *testing.T) {
	now := time.Now().Unix()
	newMute := func(id int64, muteType int) *models.AlertMute {
		return &models.AlertMute{
			Id:           id,
			GroupId:      1,
			MuteTimeType: models.TimeRange,
			Btime:        now - 100,
			Etime:        now + 100,
			MuteType:     muteType,
		}
	}
	event := &models.AlertCurEvent{GroupId: 1, TriggerTime: now, TagsMap: map[string]string{}}
	cache := &memsto.AlertMuteCacheType{}
	all := newMute(1, models.MuteTypeAll)
	notifyOnly := newMute(2, models.MuteTypeNotifyOnly)

	for _, order := range [][]*models.AlertMute{{notifyOnly, all}, {all, notifyOnly}} {
		cache.Set(map[int64][]*models.AlertMute{1: order}, 1, now)
		hit, muteId, muteType := EventMuteStrategy(event, cache)
		if !hit || muteType != models.MuteTypeAll || muteId != all.Id {
			t.Fatalf("order %v: got hit=%v muteId=%d muteType=%d, want hit=true muteId=%d muteType=%d", order, hit, muteId, muteType, all.Id, models.MuteTypeAll)
		}
	}
}
