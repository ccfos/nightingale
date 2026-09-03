package tools

import (
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
)

func TestParseDurationSeconds(t *testing.T) {
	ok := map[string]int64{
		"2h":     7200,
		"30m":    1800,
		"7d":     604800,
		"1w":     604800,
		"1d12h":  86400 + 12*3600,
		" 2H ":   7200, // trim + 大小写
		"90s":    90,
		"1d 12h": 86400 + 12*3600, // 中间空格
	}
	for in, want := range ok {
		got, err := parseDurationSeconds(in)
		if err != nil {
			t.Errorf("parseDurationSeconds(%q) unexpected err: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseDurationSeconds(%q) = %d, want %d", in, got, want)
		}
	}

	bad := []string{"", "abc", "2x", "2h5x", "0h", "-1h"}
	for _, in := range bad {
		if _, err := parseDurationSeconds(in); err == nil {
			t.Errorf("parseDurationSeconds(%q) expected error, got nil", in)
		}
	}
}

func TestNormalizeInValue(t *testing.T) {
	cases := []struct {
		in   interface{}
		want interface{}
	}{
		{[]interface{}{"web01", "web02"}, "web01 web02"},
		{[]interface{}{1, 2, 3}, "1 2 3"},
		{"web01,web02", "web01 web02"},
		{"web01, web02 , web03", "web01 web02 web03"},
		{"web01 web02", "web01 web02"}, // 已是空格分隔，原样
	}
	for _, c := range cases {
		if got := normalizeInValue(c.in); got != c.want {
			t.Errorf("normalizeInValue(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalizeMuteTags(t *testing.T) {
	// in 值传数组 → 归一成空格串；func 正常
	mute := &models.AlertMute{Tags: ormx.JSONArr(`[{"key":"ident","func":"in","value":["web01","web02"]}]`)}
	if err := normalizeMuteTags(mute); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var got []models.TagFilter
	if err := json.Unmarshal(mute.Tags, &got); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Value != "web01 web02" || got[0].Op != "in" {
		t.Errorf("normalized tags = %+v", got)
	}

	// 非法 func → 报错
	bad := &models.AlertMute{Tags: ormx.JSONArr(`[{"key":"ident","func":"contains","value":"x"}]`)}
	if err := normalizeMuteTags(bad); err == nil {
		t.Error("expected error for invalid func, got nil")
	}

	// func 缺省回退 op
	opOnly := &models.AlertMute{Tags: ormx.JSONArr(`[{"key":"ident","op":"==","value":"web01"}]`)}
	if err := normalizeMuteTags(opOnly); err != nil {
		t.Errorf("op fallback should pass, got %v", err)
	}

	// 空 tags → 不报错
	if err := normalizeMuteTags(&models.AlertMute{}); err != nil {
		t.Errorf("empty tags should pass, got %v", err)
	}
}

func TestNormalizeMutePeriodic(t *testing.T) {
	mute := &models.AlertMute{
		PeriodicMutesJson: []models.PeriodicMute{
			{EnableDaysOfWeek: "工作日", EnableStime: "02:00", EnableEtime: "06:00"},
			{EnableDaysOfWeek: "1,2,3", EnableStime: "全天", EnableEtime: ""},
			{EnableDaysOfWeek: "每天", EnableStime: "09:00", EnableEtime: "18:00"},
		},
	}
	normalizeMutePeriodic(mute)

	if mute.PeriodicMutesJson[0].EnableDaysOfWeek != "1 2 3 4 5" {
		t.Errorf("工作日 = %q", mute.PeriodicMutesJson[0].EnableDaysOfWeek)
	}
	if mute.PeriodicMutesJson[1].EnableDaysOfWeek != "1 2 3" {
		t.Errorf("逗号星期 = %q", mute.PeriodicMutesJson[1].EnableDaysOfWeek)
	}
	if mute.PeriodicMutesJson[1].EnableStime != "00:00" || mute.PeriodicMutesJson[1].EnableEtime != "23:59" {
		t.Errorf("全天 = %q ~ %q", mute.PeriodicMutesJson[1].EnableStime, mute.PeriodicMutesJson[1].EnableEtime)
	}
	if mute.PeriodicMutesJson[2].EnableDaysOfWeek != "0 1 2 3 4 5 6" {
		t.Errorf("每天 = %q", mute.PeriodicMutesJson[2].EnableDaysOfWeek)
	}
}

func TestFillMuteTime(t *testing.T) {
	// 固定时段 + duration：btime 默认 now，etime=btime+2h
	m1 := &models.AlertMute{MuteTimeType: models.TimeRange}
	if err := fillMuteTime(m1, map[string]interface{}{"duration": "2h"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m1.Btime == 0 || m1.Etime != m1.Btime+7200 {
		t.Errorf("duration fill: btime=%d etime=%d", m1.Btime, m1.Etime)
	}

	// 固定时段无 duration 无 etime → 报错
	m2 := &models.AlertMute{MuteTimeType: models.TimeRange}
	if err := fillMuteTime(m2, map[string]interface{}{}); err == nil {
		t.Error("expected error when fixed mute has no etime/duration")
	}

	// 周期屏蔽缺 etime → 默认一年
	m3 := &models.AlertMute{MuteTimeType: models.Periodic}
	if err := fillMuteTime(m3, map[string]interface{}{}); err != nil {
		t.Fatalf("periodic unexpected err: %v", err)
	}
	if m3.Etime != m3.Btime+365*24*3600 {
		t.Errorf("periodic default window: btime=%d etime=%d", m3.Btime, m3.Etime)
	}

	// 显式 btime 保留，duration 覆盖 etime
	m4 := &models.AlertMute{MuteTimeType: models.TimeRange, Btime: 1000000000, Etime: 999}
	if err := fillMuteTime(m4, map[string]interface{}{"duration": "1h"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m4.Btime != 1000000000 || m4.Etime != 1000003600 {
		t.Errorf("explicit btime + duration: btime=%d etime=%d", m4.Btime, m4.Etime)
	}
}
