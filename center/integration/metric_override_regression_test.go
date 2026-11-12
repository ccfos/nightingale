package integration

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

// File metrics regressed to overwriting database customizations with the same expression.
func TestBuiltinMetricGetsDBOverridesFileRegression(t *testing.T) {
	cache := NewBuiltinPayloadInFileType()
	cache.BuiltinMetrics["cpu"] = catalogMetric("cpu", "categraf", "Node", "%")
	customized := catalogMetric("cpu", "categraf", "Node", "ratio")
	customized.ID = 42
	customized.Translation = []models.Translation{{Lang: "en_US", Name: "Custom CPU", Note: "Custom description"}}

	metrics, total, err := cache.BuiltinMetricGets([]*models.BuiltinMetric{customized}, "en_US", "", "", "", "", 10, 0)

	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, metrics, 1)
	require.Equal(t, customized.ID, metrics[0].ID)
	require.Equal(t, "Custom CPU", metrics[0].Name)
	require.Equal(t, "Custom description", metrics[0].Note)
	require.Equal(t, "ratio", metrics[0].Unit)
}
