package sandbox

import "testing"

// TestN9eAPIEnabled pins the N9eAPI single-knob: readonly (or unset) = on, off or
// any unrecognized value (fail-safe) = off.
func TestN9eAPIEnabled(t *testing.T) {
	cases := []struct {
		preset string
		want   bool
	}{
		{"", true},
		{N9eAPIReadonly, true},
		{"ReadOnly", true},
		{N9eAPIOff, false},
		{"yolo", false},
	}
	for _, tc := range cases {
		if got := (Config{N9eAPI: tc.preset}).N9eAPIEnabled(); got != tc.want {
			t.Errorf("N9eAPI=%q: N9eAPIEnabled()=%v, want %v", tc.preset, got, tc.want)
		}
	}
}

// TestPreCheckN9eAPIDefault confirms an unset N9eAPI defaults to readonly (gateway
// on out of the box), while an explicit off is preserved.
func TestPreCheckN9eAPIDefault(t *testing.T) {
	var c Config
	c.PreCheck()
	if c.N9eAPI != N9eAPIReadonly || !c.N9eAPIEnabled() {
		t.Fatalf("PreCheck default N9eAPI=%q enabled=%v, want readonly/on", c.N9eAPI, c.N9eAPIEnabled())
	}
	off := Config{N9eAPI: N9eAPIOff}
	off.PreCheck()
	if off.N9eAPI != N9eAPIOff || off.N9eAPIEnabled() {
		t.Fatalf("explicit off clobbered: %q enabled=%v", off.N9eAPI, off.N9eAPIEnabled())
	}
}
