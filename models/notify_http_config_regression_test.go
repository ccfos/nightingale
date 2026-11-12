package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// HTTP configuration validation regressed to panicking when the entire request configuration was absent.
func TestValidateHTTPRequestConfigNilRequestRegression(t *testing.T) {
	channel := NotifyChannelConfig{RequestConfig: nil}

	require.NotPanics(t, func() {
		require.Error(t, channel.ValidateHTTPRequestConfig())
	})
}
