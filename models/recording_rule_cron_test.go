package models

import "testing"

func TestRecordingRuleVerifyCronPattern(t *testing.T) {
	cases := []struct {
		pattern string
		want    string
		wantErr bool
	}{
		{pattern: "", want: "@every 60s"},
		{pattern: "  @every 30s ", want: "@every 30s"},
		{pattern: "0 */5 * * * *", want: "0 */5 * * * *"},
		{pattern: "@hourly", want: "@hourly"},
		// 5-field form: the engine scheduler is built WithSeconds and cannot parse it
		{pattern: "*/5 * * * *", wantErr: true},
		{pattern: "not a cron", wantErr: true},
		{pattern: "@every abc", wantErr: true},
		// robfig/cron panics on these instead of returning an error
		{pattern: "TZ=UTC", wantErr: true},
		{pattern: "CRON_TZ=Asia/Shanghai", wantErr: true},
	}

	for _, c := range cases {
		re := &RecordingRule{Name: "m", PromQl: "up", CronPattern: c.pattern}
		err := re.Verify()
		if c.wantErr {
			if err == nil {
				t.Errorf("pattern %q: expected error, got nil", c.pattern)
			}
			continue
		}
		if err != nil {
			t.Errorf("pattern %q: unexpected error: %v", c.pattern, err)
			continue
		}
		if re.CronPattern != c.want {
			t.Errorf("pattern %q: got %q, want %q", c.pattern, re.CronPattern, c.want)
		}
	}
}
