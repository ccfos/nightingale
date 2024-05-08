package prom

import (
	"testing"
)

func TestAddLabelToPromQL(t *testing.T) {
	testCases := []struct {
		name     string
		label    string
		promql   string
		expected string
	}{
		{
			name:     "Add label to PromQL without existing labels",
			label:    "{new_label=\"value\"}",
			promql:   "metric_name",
			expected: "metric_name{new_label=\"value\"}",
		},
		{
			name:     "Add label to PromQL with existing labels",
			label:    "{new_label=\"value\"}",
			promql:   "metric_name{existing_label=\"value\"}",
			expected: "metric_name{new_label=\"value\",existing_label=\"value\"}",
		},
		{
			name:     "Add label with spaces to PromQL",
			label:    "{ new_label = \"value\" }",
			promql:   "metric_name",
			expected: "metric_name{new_label=\"value\"}",
		},
		{
			name:     "Add label to PromQL with multiple metrics",
			label:    "{new_label=\"value\"}",
			promql:   "metric1 + metric2{existing_label=\"value\"}",
			expected: "metric1{new_label=\"value\"} + metric2{new_label=\"value\",existing_label=\"value\"}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := AddLabelToPromQL(tc.label, tc.promql)
			if result != tc.expected {
				t.Errorf("Expected: %s, Got: %s", tc.expected, result)
			}
		})
	}
}
