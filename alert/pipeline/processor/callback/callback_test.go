package callback

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallbackConfig_Init_headers(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		want     map[string]string
	}{
		{
			name: "map headers",
			settings: map[string]any{
				"url":    "https://example.com/webhook",
				"header": map[string]string{"X-Hook-Token": "xxx"},
			},
			want: map[string]string{"X-Hook-Token": "xxx"},
		},
		{
			name: "array headers from ui",
			settings: map[string]any{
				"url": "https://example.com/webhook",
				"header": []map[string]string{
					{"key": "X-Hook-Token", "value": "xxx"},
					{"key": "X-Request-Id", "value": "abc"},
				},
			},
			want: map[string]string{
				"X-Hook-Token": "xxx",
				"X-Request-Id": "abc",
			},
		},
		{
			name: "empty array headers",
			settings: map[string]any{
				"url":    "https://example.com/webhook",
				"header": []map[string]string{},
			},
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := (&CallbackConfig{}).Init(tt.settings)
			require.NoError(t, err)
			got, ok := p.(*CallbackConfig)
			require.True(t, ok)
			assert.Equal(t, tt.want, map[string]string(got.Headers))
		})
	}
}
