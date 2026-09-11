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

func TestStripBakedThreshold(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		wantOK bool
		base   string
		op     string
		thr    float64
	}{
		{
			name: "plain baked expression", in: `kafka_messages_in_per_sec{topic="abc"} < 20000`,
			wantOK: true, base: `kafka_messages_in_per_sec{topic="abc"}`, op: "<", thr: 20000,
		},
		{
			name: "keeps original formatting of the base", in: "emqx_node_connections / (emqx_node_max_fds > 0) * 100 > 80",
			wantOK: true, base: "emqx_node_connections / (emqx_node_max_fds > 0) * 100", op: ">", thr: 80,
		},
		{
			name: "negative threshold", in: "ntp_offset_ms < -1000",
			wantOK: true, base: "ntp_offset_ms", op: "<", thr: -1000,
		},
		{name: "bare metric expression is left alone", in: "cpu_usage_active"},
		{name: "arithmetic tail is not a threshold", in: "emqx_node_connections / (emqx_node_max_fds > 0) * 100"},
		{name: "set operator on top is not strippable", in: "node_load1 > 5 and node_load5 > 3"},
		{name: "comparison against a series is not a threshold", in: "a > b"},
		{name: "aggregation is not strippable", in: "count(up == 0)"},
		{name: "unparseable input", in: "garbage !!! <<<"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base, op, thr, ok := stripBakedThreshold(c.in)
			if ok != c.wantOK {
				t.Fatalf("stripBakedThreshold(%q) ok = %v, want %v", c.in, ok, c.wantOK)
			}
			if !c.wantOK {
				return
			}
			if base != c.base || op != c.op || thr != c.thr {
				t.Fatalf("stripBakedThreshold(%q) = (%q, %q, %v), want (%q, %q, %v)",
					c.in, base, op, thr, c.base, c.op, c.thr)
			}
		})
	}
}

// The bug this whole path exists for: when the caller echoes the stored
// expression back as prom_ql, the result must equal what a plain threshold
// change produces — not a chained comparison, and not a rejection.
func TestEchoedBackPromQLBakesLikeAPlainThresholdChange(t *testing.T) {
	cases := []string{
		`kafka_messages_in_per_sec{topic="abc"} < 20000`,
		"cpu_usage_active > 80",
		"(a / b) > 0.5",
		"rate(counter[5m]) > 10",
		"emqx_node_connections / (emqx_node_max_fds > 0) * 100 > 80",
		"(tomcat_jvm_memory_total - tomcat_jvm_memory_free) / (tomcat_jvm_memory_max > 0) * 100 > 85",
	}
	for _, current := range cases {
		t.Run(current, func(t *testing.T) {
			// What the caller gets by passing only the new threshold.
			want, err := rebuildBakedPromQL(current, "", "", 42, true)
			if err != nil {
				t.Fatalf("rebuildBakedPromQL(%q) baseline: %v", current, err)
			}

			// What it gets by additionally echoing the stored expression back.
			base, bakedOp, _, ok := stripBakedThreshold(current)
			if !ok {
				t.Fatalf("stripBakedThreshold(%q) = not ok, want the expression to be recognised as baked", current)
			}
			if err := validateSimpleThresholdBase(base); err != nil {
				t.Fatalf("validateSimpleThresholdBase(%q): %v", base, err)
			}
			got, err := bakeSimpleThreshold(base, bakedOp, 42)
			if err != nil {
				t.Fatalf("bakeSimpleThreshold(%q): %v", base, err)
			}

			if got != want {
				t.Fatalf("echoed-back bake = %q, want %q", got, want)
			}
		})
	}
}

func TestValidateSimpleThresholdBase(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		wantErr bool
	}{
		{name: "bare metric", base: "cpu_usage_active"},
		{name: "selector", base: `kafka_messages_in_per_sec{topic="abc"}`},
		{name: "function", base: "rate(counter[5m])"},
		{name: "aggregation", base: "sum by (instance) (rate(x[5m]))"},
		{name: "arithmetic on top", base: "(a / b) * 100"},
		// Divide-by-zero guards keep a comparison inside a right operand or a
		// function argument. Standard PromQL, ~40 builtin templates use it.
		{name: "divide by zero guard", base: "emqx_node_connections / (emqx_node_max_fds > 0) * 100"},
		{name: "guard in the middle", base: "(tomcat_jvm_memory_total - tomcat_jvm_memory_free) / (tomcat_jvm_memory_max > 0) * 100"},
		{name: "filter inside aggregation", base: "sum(rate(x[5m]) > 0)"},
		{name: "count of a comparison", base: "count(up == 0)"},
		// Comparison on top: baking would chain a second one.
		{name: "comparison against number", base: "a < 1", wantErr: true},
		{name: "comparison against series", base: "a > b", wantErr: true},
		// Set operators bind looser than comparison, so baking would attach the
		// threshold to the right half only.
		{name: "and on top", base: "node_load1 > 5 and node_load5 > 3", wantErr: true},
		{name: "or on top", base: "a or b", wantErr: true},
		{name: "unless on top", base: "rate(errors[5m]) unless up", wantErr: true},
		{name: "empty", base: "", wantErr: true},
		{name: "unparseable", base: "garbage !!! <<<", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateSimpleThresholdBase(c.base)
			if c.wantErr && err == nil {
				t.Fatalf("validateSimpleThresholdBase(%q) = nil, want error", c.base)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("validateSimpleThresholdBase(%q) = %v, want nil", c.base, err)
			}
		})
	}
}

func TestBakeSimpleThreshold(t *testing.T) {
	got, err := bakeSimpleThreshold("cpu_usage_active", ">", 80)
	if err != nil {
		t.Fatalf("bakeSimpleThreshold: %v", err)
	}
	if want := "cpu_usage_active > 80"; got != want {
		t.Fatalf("bakeSimpleThreshold = %q, want %q", got, want)
	}

	// Regression: the stripped branch bypasses rebuildBakedPromQL, which used to
	// be the only place the operator was checked. An unchecked operator would be
	// concatenated straight into the expression and stored.
	if _, err := bakeSimpleThreshold(`metric{a="b"}`, "bogus", 42); err == nil {
		t.Fatal("bakeSimpleThreshold with an invalid operator = nil, want error")
	}
	if _, err := bakeSimpleThreshold("", ">", 1); err == nil {
		t.Fatal("bakeSimpleThreshold with an empty base = nil, want error")
	}
}
