package doris

import "testing"

func TestResolveReadAddr(t *testing.T) {
	tests := []struct {
		name       string
		isCenter   bool
		centerAddr string
		localAddr  string
		want       string
	}{
		{name: "center ignores local", isCenter: true, centerAddr: "pub:9030", localAddr: "local:9030", want: "pub:9030"},
		{name: "center empty local", isCenter: true, centerAddr: "pub:9030", localAddr: "", want: "pub:9030"},
		{name: "edge uses local", isCenter: false, centerAddr: "pub:9030", localAddr: "local:9030", want: "local:9030"},
		{name: "edge fallback when empty", isCenter: false, centerAddr: "pub:9030", localAddr: "", want: "pub:9030"},
		{name: "edge trims local", isCenter: false, centerAddr: "pub:9030", localAddr: "  local:9030  ", want: "local:9030"},
		{name: "edge whitespace-only local falls back", isCenter: false, centerAddr: "pub:9030", localAddr: "   ", want: "pub:9030"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveReadAddr(tt.isCenter, tt.centerAddr, tt.localAddr)
			if got != tt.want {
				t.Fatalf("ResolveReadAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyReadAddr(t *testing.T) {
	d := &Doris{Addr: "pub:9030", InternalAddr: "local:9030", FeAddr: "local:8030"}
	if used := d.ApplyReadAddr(true); used {
		t.Fatalf("center should not use local")
	}
	if d.Addr != "pub:9030" || d.FeAddr != "local:8030" {
		t.Fatalf("center rewrite mutated unexpectedly: %+v", d)
	}

	d = &Doris{Addr: "pub:9030", InternalAddr: "local:9030", FeAddr: "local:8030"}
	if used := d.ApplyReadAddr(false); !used {
		t.Fatalf("edge with local should report usedLocal")
	}
	if d.Addr != "local:9030" {
		t.Fatalf("edge Addr = %q, want local:9030", d.Addr)
	}
	if d.FeAddr != "local:8030" {
		t.Fatalf("FeAddr must stay untouched, got %q", d.FeAddr)
	}

	d = &Doris{Addr: "pub:9030", InternalAddr: "", FeAddr: "local:8030"}
	if used := d.ApplyReadAddr(false); used {
		t.Fatalf("edge without local should not report usedLocal")
	}
	if d.Addr != "pub:9030" {
		t.Fatalf("edge fallback Addr = %q, want pub:9030", d.Addr)
	}
}
