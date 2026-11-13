package integration

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func catalogMetric(expression, collector, typ, unit string) *models.BuiltinMetric {
	return &models.BuiltinMetric{
		Expression: expression, Collector: collector, Typ: typ, Unit: unit, Name: expression, Note: "system usage",
		Translation: []models.Translation{{Lang: "en_US", Name: "English " + expression, Note: "English note"}},
	}
}

func TestGetComponentIdentByCate_ResolvesSourceCategory(t *testing.T) {
	cache := NewBuiltinPayloadInFileType()
	cache.AddBuiltinPayload(&models.BuiltinPayload{
		UUID: 1, ComponentID: 1, Component: "Node", Type: "alert", Cate: "cpu", Name: "CPU usage",
	})

	ident := cache.GetComponentIdentByCate("alert", "cpu")

	require.Equal(t, "Node", ident)
	require.Empty(t, cache.GetComponentIdentByCate("dashboard", "cpu"))
	require.Empty(t, cache.GetComponentIdentByCate("alert", "missing"))
}

func TestBuiltinMetricTypes_FiltersAndDeduplicatesTypes(t *testing.T) {
	cache := NewBuiltinPayloadInFileType()
	cache.BuiltinMetrics = map[string]*models.BuiltinMetric{
		"cpu": catalogMetric("cpu", "categraf", "Node", "%"),
		"mem": catalogMetric("mem", "categraf", "Node", "bytes"),
		"sql": catalogMetric("sql", "sql_exporter", "MySQL", "ms"),
	}

	types := cache.BuiltinMetricTypes("en_US", "categraf", "usage")

	require.Equal(t, []string{"Node"}, types)
	require.Empty(t, cache.BuiltinMetricTypes("en_US", "categraf", "missing"))
}

func TestBuiltinMetricCollectors_FiltersAndDeduplicatesCollectors(t *testing.T) {
	cache := NewBuiltinPayloadInFileType()
	cache.BuiltinMetrics = map[string]*models.BuiltinMetric{
		"cpu": catalogMetric("cpu", "categraf", "Node", "%"),
		"mem": catalogMetric("mem", "categraf", "Node", "bytes"),
		"sql": catalogMetric("sql", "sql_exporter", "MySQL", "ms"),
	}

	collectors := cache.BuiltinMetricCollectors("en_US", "Node", "usage")

	require.Equal(t, []string{"categraf"}, collectors)
	require.Empty(t, cache.BuiltinMetricCollectors("en_US", "MySQL", "missing"))
}

func TestBuiltinMetricGets_SortsBeforePaginationAndFallsBackToEnglish(t *testing.T) {
	cache := NewBuiltinPayloadInFileType()
	cache.BuiltinMetrics = map[string]*models.BuiltinMetric{
		"z": catalogMetric("z", "b", "Node", "%"),
		"b": catalogMetric("b", "a", "Node", "%"),
		"a": catalogMetric("a", "a", "Node", "%"),
		"c": catalogMetric("c", "a", "SQL", "%"),
	}

	metrics, total, err := cache.BuiltinMetricGets(nil, "fr_FR", "", "", "", "", 2, 1)

	require.NoError(t, err)
	require.Equal(t, 4, total)
	require.Len(t, metrics, 2)
	require.Equal(t, "b", metrics[0].Expression)
	require.Equal(t, "c", metrics[1].Expression)
	require.Equal(t, "English b", metrics[0].Name)
	require.Equal(t, "English note", metrics[0].Note)
}

func TestBuiltinMetricGets_NormalizesPaginationBounds(t *testing.T) {
	cache := NewBuiltinPayloadInFileType()
	cache.BuiltinMetrics["cpu"] = catalogMetric("cpu", "categraf", "Node", "%")
	tests := []struct {
		name   string
		limit  int
		offset int
		length int
	}{
		{name: "negative offset", limit: 10, offset: -1, length: 1},
		{name: "negative limit", limit: -1, length: 0},
		{name: "zero limit", limit: 0, length: 0},
		{name: "past end", limit: 10, offset: 1, length: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, total, err := cache.BuiltinMetricGets(nil, "en_US", "", "", "", "", tt.limit, tt.offset)

			require.NoError(t, err)
			require.Equal(t, 1, total)
			require.Len(t, metrics, tt.length)
		})
	}
}

func TestBuiltinMetricGets_AppliesCollectorTypeUnitAndQueryFilters(t *testing.T) {
	cache := NewBuiltinPayloadInFileType()
	cache.BuiltinMetrics = map[string]*models.BuiltinMetric{
		"cpu": catalogMetric("cpu", "categraf", "Node", "%"),
		"mem": catalogMetric("mem", "categraf", "Node", "bytes"),
		"sql": catalogMetric("sql", "sql_exporter", "MySQL", "ms"),
	}
	tests := []struct {
		name      string
		collector string
		typ       string
		query     string
		unit      string
		want      []string
	}{
		{name: "collector", collector: "sql_exporter", want: []string{"sql"}},
		{name: "type", typ: "MySQL", want: []string{"sql"}},
		{name: "units", unit: "bytes,ms", want: []string{"mem", "sql"}},
		{name: "unit exact match", unit: "byte"},
		{name: "all positive terms", query: "cpu usage", want: []string{"cpu"}},
		{name: "exclude term", query: "usage -mem", want: []string{"cpu", "sql"}},
		{name: "missing positive term", query: "cpu missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, total, err := cache.BuiltinMetricGets(nil, "en_US", tt.collector, tt.typ, tt.query, tt.unit, 10, 0)

			require.NoError(t, err)
			var expressions []string
			for _, metric := range metrics {
				expressions = append(expressions, metric.Expression)
			}
			require.ElementsMatch(t, tt.want, expressions)
			require.Equal(t, len(tt.want), total)
		})
	}
}

func TestBuiltinMetricGets_SkipsMetricsWithoutRequestedOrEnglishTranslation(t *testing.T) {
	cache := NewBuiltinPayloadInFileType()
	cache.BuiltinMetrics["cpu"] = &models.BuiltinMetric{
		Expression: "cpu", Translation: []models.Translation{{Lang: "zh_CN", Name: "CPU"}},
	}

	metrics, total, err := cache.BuiltinMetricGets(nil, "fr_FR", "", "", "", "", 10, 0)

	require.NoError(t, err)
	require.Empty(t, metrics)
	require.Zero(t, total)
}

func TestBuiltinMetricGets_MergesLegacyTranslationsIntoLowestID(t *testing.T) {
	cache := NewBuiltinPayloadInFileType()
	metricsInDB := []*models.BuiltinMetric{
		{ID: 20, Expression: "cpu", Lang: "en_US", Name: "CPU usage", Note: "English note"},
		{ID: 10, Expression: "cpu", Lang: "zh_CN", Name: "CPU 使用率", Note: "中文说明"},
	}

	metrics, total, err := cache.BuiltinMetricGets(metricsInDB, "en_US", "", "", "", "", 10, 0)

	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, metrics, 1)
	require.Equal(t, int64(10), metrics[0].ID)
	require.Equal(t, "CPU usage", metrics[0].Name)
	require.Equal(t, "English note", metrics[0].Note)
	require.ElementsMatch(t, []models.Translation{
		{Lang: "zh_CN", Name: "CPU 使用率", Note: "中文说明"},
		{Lang: "en_US", Name: "CPU usage", Note: "English note"},
	}, metrics[0].Translation)
}

func TestBuiltinMetricGets_PreservesCustomizedTranslationOnPrimaryRecord(t *testing.T) {
	cache := NewBuiltinPayloadInFileType()
	metricsInDB := []*models.BuiltinMetric{
		{ID: 20, Expression: "cpu", Lang: "en_US", Name: "Legacy name"},
		{
			ID: 10, Expression: "cpu",
			Translation: []models.Translation{{Lang: "en_US", Name: "Customized", Note: "User note"}},
		},
	}

	metrics, total, err := cache.BuiltinMetricGets(metricsInDB, "en_US", "", "", "", "", 10, 0)

	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, metrics, 1)
	require.Equal(t, "Customized", metrics[0].Name)
	require.Equal(t, "User note", metrics[0].Note)
	require.Len(t, metrics[0].Translation, 1)
}
