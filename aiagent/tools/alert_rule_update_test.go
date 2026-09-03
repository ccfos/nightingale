package tools

import "testing"

func TestRebuildBakedPromQL(t *testing.T) {
	cases := []struct {
		name         string
		current      string
		newBase      string
		newOp        string
		newThreshold float64
		hasThreshold bool
		want         string
		wantErr      bool
	}{
		{
			name:         "threshold only — keep base and operator",
			current:      `cpu_usage_active{cpu="cpu-total"} > 80`,
			newThreshold: 20,
			hasThreshold: true,
			want:         `cpu_usage_active{cpu="cpu-total"} > 20`,
		},
		{
			name:    "operator only — keep base and threshold",
			current: "x > 80",
			newOp:   ">=",
			want:    "x >= 80",
		},
		{
			name:         "wrapped base is preserved verbatim (no double-wrap)",
			current:      "(a / b) > 0.5",
			newThreshold: 0.8,
			hasThreshold: true,
			want:         "(a / b) > 0.8",
		},
		{
			name:         ">= wins over > in operator parsing",
			current:      "mem >= 90",
			newThreshold: 50,
			hasThreshold: true,
			want:         "mem >= 50",
		},
		{
			name:         "new complex base gets wrapped",
			current:      "x > 80",
			newBase:      "a/b",
			newThreshold: 5,
			hasThreshold: true,
			want:         "(a/b) > 5",
		},
		{
			name:         "unparseable current + new base + threshold → rebuilt",
			current:      "garbage_no_operator",
			newBase:      "cpu",
			newThreshold: 10,
			hasThreshold: true,
			want:         "cpu > 10",
		},
		{
			name:    "unparseable current, nothing to keep → error",
			current: "garbage_no_operator",
			wantErr: true,
		},
		{
			name:    "empty current, no overrides → error",
			current: "",
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := rebuildBakedPromQL(c.current, c.newBase, c.newOp, c.newThreshold, c.hasThreshold)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestCurrentBakedPromQLAndPromQueries(t *testing.T) {
	// Mirror the shape DB2FE produces: json.Unmarshal into interface{} yields
	// map[string]interface{} with queries as []interface{} of maps.
	rc := map[string]interface{}{
		"queries": []interface{}{
			map[string]interface{}{"prom_ql": "up > 0", "severity": float64(2)},
		},
	}
	if got := currentBakedPromQL(rc); got != "up > 0" {
		t.Fatalf("currentBakedPromQL = %q, want %q", got, "up > 0")
	}

	qs, ok := promQueries(rc)
	if !ok || len(qs) != 1 {
		t.Fatalf("promQueries ok=%v len=%d, want ok=true len=1", ok, len(qs))
	}
	// Mutating the returned map must write through to rc (aliasing contract).
	qs[0]["prom_ql"] = "up > 1"
	if got := currentBakedPromQL(rc); got != "up > 1" {
		t.Fatalf("after mutation currentBakedPromQL = %q, want %q", got, "up > 1")
	}

	for _, bad := range []interface{}{
		nil,
		"not a map",
		map[string]interface{}{}, // no queries
		map[string]interface{}{"queries": []interface{}{}},       // empty
		map[string]interface{}{"queries": []interface{}{"oops"}}, // non-map element
	} {
		if _, ok := promQueries(bad); ok {
			t.Fatalf("promQueries(%v) ok=true, want false", bad)
		}
		if got := currentBakedPromQL(bad); got != "" {
			t.Fatalf("currentBakedPromQL(%v) = %q, want empty", bad, got)
		}
	}
}

func TestApplyRuleConfigSeverity(t *testing.T) {
	// prometheus keeps severity per query.
	prom := map[string]interface{}{
		"queries": []interface{}{
			map[string]interface{}{"prom_ql": "up > 0", "severity": float64(2)},
		},
	}
	applyRuleConfigSeverity(prom, 1)
	if got := prom["queries"].([]interface{})[0].(map[string]interface{})["severity"]; got != 1 {
		t.Fatalf("prometheus query severity = %v, want 1", got)
	}

	// host / other cate types keep severity per trigger — issue #4: this must be
	// synced too, not just prometheus.
	host := map[string]interface{}{
		"triggers": []interface{}{
			map[string]interface{}{"severity": float64(3)},
			map[string]interface{}{"severity": float64(3)},
		},
	}
	applyRuleConfigSeverity(host, 2)
	for i, e := range host["triggers"].([]interface{}) {
		if got := e.(map[string]interface{})["severity"]; got != 2 {
			t.Fatalf("trigger[%d] severity = %v, want 2", i, got)
		}
	}

	// Non-map / missing arrays must be a no-op (and must not panic).
	applyRuleConfigSeverity(nil, 1)
	applyRuleConfigSeverity("not a map", 1)
	applyRuleConfigSeverity(map[string]interface{}{}, 1)
}
