package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEsIndexPatternAdd_EnforcesDatasourceNameUniquenessAndEncodesVisibility(t *testing.T) {
	c := pipelineContext(t, &EsIndexPattern{})
	pattern := EsIndexPattern{DatasourceId: 7, Name: "logs-*", AllowHideSystemIndicesBool: true}
	require.NoError(t, pattern.Add(c))
	duplicate := EsIndexPattern{DatasourceId: 7, Name: "logs-*"}
	other := EsIndexPattern{DatasourceId: 8, Name: "logs-*"}

	err := duplicate.Add(c)

	require.ErrorContains(t, err, "already exists")
	require.NoError(t, other.Add(c))
	got, err := EsIndexPatternGetById(c, pattern.Id)
	require.NoError(t, err)
	require.Equal(t, 1, got.AllowHideSystemIndices)
	require.True(t, got.AllowHideSystemIndicesBool)
	missing, err := EsIndexPatternGetById(c, 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestEsIndexPatternUpdate_RejectsConflictingDatasourceName(t *testing.T) {
	c := pipelineContext(t, &EsIndexPattern{})
	original := EsIndexPattern{Id: 1, DatasourceId: 7, Name: "original"}
	require.NoError(t, c.DB.Create(&original).Error)
	require.NoError(t, c.DB.Create(&EsIndexPattern{Id: 2, DatasourceId: 7, Name: "taken"}).Error)

	err := original.Update(c, EsIndexPattern{DatasourceId: 7, Name: "taken"})

	require.ErrorContains(t, err, "already exists")
	got, err := EsIndexPatternGetById(c, 1)
	require.NoError(t, err)
	require.Equal(t, "original", got.Name)
	require.NoError(t, original.Update(c, EsIndexPattern{DatasourceId: 8, Name: "taken"}))
	got, err = EsIndexPatternGetById(c, 1)
	require.NoError(t, err)
	require.Equal(t, int64(8), got.DatasourceId)
	require.Equal(t, "taken", got.Name)
}

func TestGetAlertRulesByEsIndexPatternId_RejectsPrefixMatchesAndMalformedJSON(t *testing.T) {
	c := pipelineContext(t, &EsIndexPattern{}, &AlertRule{})
	require.NoError(t, c.DB.Create(&[]AlertRule{
		{Id: 1, Name: "exact", RuleConfig: `{"queries":[{"index_pattern":7}]}`},
		{Id: 2, Name: "prefix", RuleConfig: `{"queries":[{"index_pattern":70}]}`},
		{Id: 3, Name: "malformed", RuleConfig: `{"queries":[{"index_pattern":7`},
		{Id: 4, Name: "unrelated", RuleConfig: `{"queries":[{"index_pattern":8}]}`},
	}).Error)

	rules, err := GetAlertRulesByEsIndexPatternId(c, 7)

	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, "exact", rules[0].Name)
}

func TestEsIndexPatternDel_ProtectsReferencedPatternsBeforeDeletingAnyRows(t *testing.T) {
	c := pipelineContext(t, &EsIndexPattern{}, &AlertRule{})
	require.NoError(t, c.DB.Create(&[]EsIndexPattern{{Id: 7, Name: "used"}, {Id: 8, Name: "free"}}).Error)
	require.NoError(t, c.DB.Create(&AlertRule{Name: "CPU", RuleConfig: `{"queries":[{"index_pattern":7}]}`}).Error)

	err := EsIndexPatternDel(c, []int64{8, 7})

	require.ErrorContains(t, err, "used by alert rules: CPU")
	retained, err := EsIndexPatternGetById(c, 8)
	require.NoError(t, err)
	require.NotNil(t, retained)
	require.NoError(t, EsIndexPatternDel(c, []int64{8}))
	missing, err := EsIndexPatternGetById(c, 8)
	require.NoError(t, err)
	require.Nil(t, missing)
	require.NoError(t, EsIndexPatternDel(c, nil))
}
