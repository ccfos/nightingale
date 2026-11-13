package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuiltinComponentAdd_TrimsIdentAndRejectsDuplicates(t *testing.T) {
	c := pipelineContext(t, &BuiltinComponent{})
	component := BuiltinComponent{ID: 1, Ident: "  redis  ", Readme: "Redis metrics"}

	require.NoError(t, component.Add(c, "author"))
	duplicate := BuiltinComponent{Ident: "redis"}

	require.EqualError(t, duplicate.Add(c, "other"), "builtin component already exists")
	blank := BuiltinComponent{Ident: " \t "}
	require.EqualError(t, blank.Add(c, "author"), "ident is blank")
	stored, err := BuiltinComponentGet(c, "id = ?", component.ID)
	require.NoError(t, err)
	require.Equal(t, "redis", stored.Ident)
	require.Equal(t, "author", stored.CreatedBy)
	require.Equal(t, "author", stored.UpdatedBy)
	missing, err := BuiltinComponentGet(c, "id = ?", 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestBuiltinComponentUpdate_RejectsConflictingIdent(t *testing.T) {
	c := pipelineContext(t, &BuiltinComponent{})
	component := BuiltinComponent{ID: 1, Ident: "redis"}
	require.NoError(t, c.DB.Create(&component).Error)
	require.NoError(t, c.DB.Create(&BuiltinComponent{ID: 2, Ident: "mysql"}).Error)

	err := component.Update(c, BuiltinComponent{Ident: "mysql"})

	require.EqualError(t, err, "builtin component already exists")
	require.NoError(t, component.Update(c, BuiltinComponent{
		ID: 1, Ident: "postgres", Readme: "updated", UpdatedBy: "editor",
	}))
	stored, err := BuiltinComponentGet(c, "id = ?", 1)
	require.NoError(t, err)
	require.Equal(t, "postgres", stored.Ident)
	require.Equal(t, "updated", stored.Readme)
	require.Equal(t, "editor", stored.UpdatedBy)
}

func TestBuiltinComponentUpdateDisabled_PreservesSystemOwnershipAndContent(t *testing.T) {
	c := pipelineContext(t, &BuiltinComponent{})
	component := BuiltinComponent{ID: 1, Ident: "redis", Readme: "system readme", UpdatedBy: SYSTEM, UpdatedAt: 1}
	require.NoError(t, c.DB.Create(&component).Error)

	require.NoError(t, component.UpdateDisabled(c, 1))
	stored, err := BuiltinComponentGet(c, "id = ?", component.ID)

	require.NoError(t, err)
	require.Equal(t, 1, stored.Disabled)
	require.Equal(t, SYSTEM, stored.UpdatedBy)
	require.Equal(t, "system readme", stored.Readme)
	require.NoError(t, component.UpdateDisabled(c, 0))
	stored, err = BuiltinComponentGet(c, "id = ?", component.ID)
	require.NoError(t, err)
	require.Zero(t, stored.Disabled)
}

func TestBuiltinComponentGets_FiltersAndOrdersByAvailabilityRecencyAndIdent(t *testing.T) {
	c := pipelineContext(t, &BuiltinComponent{})
	require.NoError(t, c.DB.Create(&[]BuiltinComponent{
		{ID: 1, Ident: "db-old", UpdatedAt: 100},
		{ID: 2, Ident: "db-disabled", Disabled: 1, UpdatedAt: 900},
		{ID: 3, Ident: "db-b", UpdatedAt: 200},
		{ID: 4, Ident: "db-a", UpdatedAt: 200},
		{ID: 5, Ident: "other", UpdatedAt: 300},
	}).Error)

	components, err := BuiltinComponentGets(c, "db-", -1)

	require.NoError(t, err)
	require.Len(t, components, 4)
	idents := make([]string, 0, len(components))
	for _, component := range components {
		idents = append(idents, component.Ident)
	}
	require.Equal(t, []string{"db-a", "db-b", "db-old", "db-disabled"}, idents)
	disabled, err := BuiltinComponentGets(c, "", 1)
	require.NoError(t, err)
	require.Len(t, disabled, 1)
	require.Equal(t, uint64(2), disabled[0].ID)
}

func TestBuiltinComponentDels_DeletesOnlyRequestedIDs(t *testing.T) {
	c := pipelineContext(t, &BuiltinComponent{})
	require.NoError(t, c.DB.Create(&[]BuiltinComponent{{ID: 1, Ident: "retained"}, {ID: 2, Ident: "deleted"}}).Error)

	require.NoError(t, BuiltinComponentDels(c, nil))
	require.NoError(t, BuiltinComponentDels(c, []int64{2}))

	components, err := BuiltinComponentGets(c, "", -1)
	require.NoError(t, err)
	require.Len(t, components, 1)
	require.Equal(t, uint64(1), components[0].ID)
}

func TestBuiltinPayloadVerify_RequiresTypeComponentAndName(t *testing.T) {
	cases := []struct {
		name    string
		payload BuiltinPayload
		want    string
	}{
		{name: "type", payload: BuiltinPayload{Type: " \t "}, want: "type is blank"},
		{name: "component", payload: BuiltinPayload{Type: "Alert"}, want: "component_id is blank"},
		{name: "name", payload: BuiltinPayload{Type: "Alert", ComponentID: 1}, want: "name is blank"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.payload.Verify()

			require.EqualError(t, err, tc.want)
		})
	}
}

func TestBuiltinPayloadAdd_RejectsDuplicateWithinTypeComponentAndCategory(t *testing.T) {
	c := pipelineContext(t, &BuiltinPayload{})
	payload := BuiltinPayload{ID: 1, Type: " Alert ", ComponentID: 7, Cate: "categraf", Name: "CPU", Content: "{}"}
	require.NoError(t, payload.Add(c, "author"))
	duplicate := BuiltinPayload{Type: "Alert", ComponentID: 7, Cate: "categraf", Name: "CPU"}

	err := duplicate.Add(c, "editor")

	require.EqualError(t, err, "builtin payload already exists")
	duplicate.Cate = "telegraf"
	require.NoError(t, duplicate.Add(c, "editor"))
	stored, err := BuiltinPayloadGet(c, "id = ?", payload.ID)
	require.NoError(t, err)
	require.Equal(t, "Alert", stored.Type)
	require.Equal(t, "author", stored.CreatedBy)
	require.Equal(t, "author", stored.UpdatedBy)
	missing, err := BuiltinPayloadGet(c, "id = ?", 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestBuiltinPayloadUpdate_PreservesUUIDAndCreationMetadata(t *testing.T) {
	c := pipelineContext(t, &BuiltinPayload{})
	payload := BuiltinPayload{
		ID: 1, Type: "Alert", ComponentID: 7, Name: "CPU", UUID: 42, CreatedAt: 100, CreatedBy: "author",
	}
	require.NoError(t, c.DB.Create(&payload).Error)
	replacement := BuiltinPayload{
		ID: 1, Type: "Alert", ComponentID: 7, Name: "Memory", Content: "updated", UUID: 99,
		CreatedAt: 999, CreatedBy: "untrusted", UpdatedBy: "editor",
	}

	require.NoError(t, payload.Update(c, replacement))
	stored, err := BuiltinPayloadGet(c, "id = ?", 1)

	require.NoError(t, err)
	require.Equal(t, "Memory", stored.Name)
	require.Equal(t, "updated", stored.Content)
	require.Equal(t, int64(42), stored.UUID)
	require.Equal(t, int64(100), stored.CreatedAt)
	require.Equal(t, "author", stored.CreatedBy)
	require.Equal(t, "editor", stored.UpdatedBy)
}

func TestBuiltinPayloadQueries_ExcludeSystemRowsAndCombineFilters(t *testing.T) {
	c := pipelineContext(t, &BuiltinPayload{})
	require.NoError(t, c.DB.Create(&[]BuiltinPayload{
		{ID: 1, Type: "Alert", ComponentID: 7, Component: "redis", Cate: "categraf", Name: "CPU", Tags: "prod"},
		{ID: 2, Type: "Alert", ComponentID: 7, Component: "redis", Cate: "categraf", Name: "CPU", UpdatedBy: SYSTEM},
		{ID: 3, Type: "Dashboard", ComponentID: 7, Cate: "categraf", Name: "CPU", Tags: "prod"},
		{ID: 4, Type: "Alert", ComponentID: 8, Cate: "telegraf", Name: "CPU", Tags: "prod"},
	}).Error)

	payloads, err := BuiltinPayloadGets(c, 7, "Alert", "categraf", "CPU prod")

	require.NoError(t, err)
	require.Len(t, payloads, 1)
	require.Equal(t, int64(1), payloads[0].ID)
	cates, err := BuiltinPayloadCates(c, "Alert", 7)
	require.NoError(t, err)
	require.Equal(t, []string{"categraf"}, cates)
	component, err := BuiltinPayloadComponents(c, "Alert", "categraf")
	require.NoError(t, err)
	require.Equal(t, "redis", component)
	component, err = BuiltinPayloadComponents(c, "missing", "categraf")
	require.NoError(t, err)
	require.Empty(t, component)
}

func TestBuiltinPayloadDels_DeletesOnlyRequestedIDs(t *testing.T) {
	c := pipelineContext(t, &BuiltinPayload{})
	require.NoError(t, c.DB.Create(&[]BuiltinPayload{
		{ID: 1, Type: "Alert", ComponentID: 7, Name: "deleted"},
		{ID: 2, Type: "Alert", ComponentID: 7, Name: "retained"},
	}).Error)

	require.NoError(t, BuiltinPayloadDels(c, nil))
	require.NoError(t, BuiltinPayloadDels(c, []int64{1}))

	payload, err := BuiltinPayloadGet(c, "id = ?", 1)
	require.NoError(t, err)
	require.Nil(t, payload)
	payload, err = BuiltinPayloadGet(c, "id = ?", 2)
	require.NoError(t, err)
	require.Equal(t, "retained", payload.Name)
}

func TestInitBuiltinPayloads_ResolvesLegacyComponentNames(t *testing.T) {
	c := pipelineContext(t, &BuiltinPayload{}, &BuiltinComponent{})
	require.NoError(t, c.DB.Create(&BuiltinComponent{ID: 7, Ident: "redis"}).Error)
	require.NoError(t, c.DB.Create(&[]BuiltinPayload{
		{ID: 1, Component: "redis"},
		{ID: 2, Component: "unknown"},
		{ID: 3, Component: "redis", ComponentID: 8},
	}).Error)

	require.NoError(t, InitBuiltinPayloads(c))

	for id, want := range map[int64]uint64{1: 7, 2: 0, 3: 8} {
		payload, err := BuiltinPayloadGet(c, "id = ?", id)
		require.NoError(t, err)
		require.Equal(t, want, payload.ComponentID)
	}
}

func TestBuiltinMetricVerify_RequiresTranslationCollectorAndType(t *testing.T) {
	cases := []struct {
		name   string
		metric BuiltinMetric
		want   string
	}{
		{name: "translation", metric: BuiltinMetric{}, want: "translation is required"},
		{
			name: "collector", metric: BuiltinMetric{Translation: []Translation{{Lang: "en"}}, Collector: " "},
			want: "collector is blank",
		},
		{
			name: "type", metric: BuiltinMetric{Translation: []Translation{{Lang: "en"}}, Collector: "categraf", Typ: " "},
			want: "type is blank",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.metric.Verify()

			require.EqualError(t, err, tc.want)
		})
	}
}

func TestBuiltinMetricAdd_UsesExpressionCollectorAndTypeForUniqueness(t *testing.T) {
	c := pipelineContext(t, &BuiltinMetric{})
	metric := BuiltinMetric{
		ID: 1, Collector: " categraf ", Typ: " cpu ", Expression: "cpu_usage", Name: "CPU",
		Translation: []Translation{{Lang: "en", Name: "CPU", Note: "CPU usage"}},
	}
	require.NoError(t, metric.Add(c, "author"))
	duplicate := metric
	duplicate.ID = 2
	duplicate.Name = "Different display name"

	err := duplicate.Add(c, "editor")

	require.EqualError(t, err, "builtin metric already exists")
	stored, err := BuiltinMetricGet(c, "id = ?", 1)
	require.NoError(t, err)
	require.Equal(t, "categraf", stored.Collector)
	require.Equal(t, "cpu", stored.Typ)
	require.Equal(t, metric.Translation, stored.Translation)
	require.Equal(t, "author", stored.CreatedBy)
	require.Equal(t, "author", stored.UpdatedBy)
}

func TestBuiltinMetricUpdate_PreservesUUIDAndCreationMetadata(t *testing.T) {
	c := pipelineContext(t, &BuiltinMetric{})
	metric := BuiltinMetric{ID: 1, UUID: 42, CreatedAt: 100, CreatedBy: "author"}
	require.NoError(t, c.DB.Create(&metric).Error)
	replacement := BuiltinMetric{
		Collector: "categraf", Typ: "cpu", Expression: "cpu_usage", Translation: []Translation{{Lang: "en", Name: "CPU"}},
		ID: 1, UUID: 99, CreatedAt: 999, CreatedBy: "untrusted", UpdatedBy: "editor",
	}

	require.NoError(t, metric.Update(c, replacement))
	stored, err := BuiltinMetricGet(c, "id = ?", 1)

	require.NoError(t, err)
	require.Equal(t, int64(42), stored.UUID)
	require.Equal(t, int64(100), stored.CreatedAt)
	require.Equal(t, "author", stored.CreatedBy)
	require.Equal(t, "editor", stored.UpdatedBy)
	require.Equal(t, "cpu_usage", stored.Expression)
}

func TestBuiltinMetricGets_CombinesMetadataUnitsAndSearchTerms(t *testing.T) {
	c := pipelineContext(t, &BuiltinMetric{})
	require.NoError(t, c.DB.Create(&[]BuiltinMetric{
		{
			ID: 1, Lang: "en", Collector: "categraf", Typ: "cpu", Unit: "%",
			Name: "Usage", Expression: "cpu_usage", Note: "prod",
		},
		{
			ID: 2, Lang: "en", Collector: "categraf", Typ: "cpu", Unit: "%",
			Name: "Legacy", Expression: "cpu_usage", Note: "prod",
		},
		{ID: 3, Lang: "en", Collector: "categraf", Typ: "cpu", Unit: "%", Name: "Usage", UpdatedBy: SYSTEM},
		{
			ID: 4, Lang: "zh", Collector: "categraf", Typ: "cpu", Unit: "%",
			Name: "Usage", Expression: "cpu_usage", Note: "prod",
		},
		{ID: 5, Lang: "en", Collector: "other", Typ: "cpu", Unit: "%", Name: "Usage", Expression: "cpu_usage", Note: "prod"},
	}).Error)

	metrics, err := BuiltinMetricGets(c, "en", "categraf", "cpu", "cpu prod -Legacy", "%,bytes")

	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, int64(1), metrics[0].ID)
	types, err := BuiltinMetricTypes(c, "en", "categraf", "cp")
	require.NoError(t, err)
	require.Equal(t, []string{"cpu"}, types)
	collectors, err := BuiltinMetricCollectors(c, "en", "cpu", "cate")
	require.NoError(t, err)
	require.Equal(t, []string{"categraf"}, collectors)
}

func TestBuiltinMetricBatchUpdateColumn_UpdatesOnlyMatchingRows(t *testing.T) {
	c := pipelineContext(t, &BuiltinMetric{})
	require.NoError(t, c.DB.Create(&[]BuiltinMetric{
		{ID: 1, Collector: "old"},
		{ID: 2, Collector: "other"},
	}).Error)

	require.NoError(t, BuiltinMetricBatchUpdateColumn(c, "collector", "old", "new", "editor"))
	require.NoError(t, BuiltinMetricBatchUpdateColumn(c, "collector", "new", "new", "ignored"))

	stored, err := BuiltinMetricGet(c, "id = ?", 1)
	require.NoError(t, err)
	require.Equal(t, "new", stored.Collector)
	require.Equal(t, "editor", stored.UpdatedBy)
	other, err := BuiltinMetricGet(c, "id = ?", 2)
	require.NoError(t, err)
	require.Equal(t, "other", other.Collector)
	require.Empty(t, other.UpdatedBy)
}

func TestBuiltinMetricDels_DeletesOnlyRequestedIDs(t *testing.T) {
	c := pipelineContext(t, &BuiltinMetric{})
	require.NoError(t, c.DB.Create(&[]BuiltinMetric{{ID: 1, Name: "deleted"}, {ID: 2, Name: "retained"}}).Error)

	require.NoError(t, BuiltinMetricDels(c, nil))
	require.NoError(t, BuiltinMetricDels(c, []int64{1}))

	missing, err := BuiltinMetricGet(c, "id = ?", 1)
	require.NoError(t, err)
	require.Nil(t, missing)
	retained, err := BuiltinMetricGet(c, "id = ?", 2)
	require.NoError(t, err)
	require.Equal(t, "retained", retained.Name)
}
