package mute

import (
	"testing"
	"time"

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
