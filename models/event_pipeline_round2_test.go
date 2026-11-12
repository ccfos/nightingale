package models

import (
	"fmt"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
)

func TestEventPipelineVerify_RequiresNameAndOwnership(t *testing.T) {
	cases := []struct {
		name     string
		pipeline EventPipeline
		want     string
	}{
		{name: "missing name", pipeline: EventPipeline{GroupId: 7}, want: "name cannot be empty"},
		{name: "missing owner", pipeline: EventPipeline{Name: "pipeline"}, want: "team_ids cannot be empty"},
		{name: "team owner", pipeline: EventPipeline{Name: "pipeline", TeamIds: []int64{7}}},
		{name: "business group owner", pipeline: EventPipeline{Name: "pipeline", GroupId: 7}},
		{name: "alert rule use case", pipeline: EventPipeline{Name: "pipeline", UseCase: UseCaseAlertRule}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.pipeline.Verify()

			if tc.want != "" {
				require.EqualError(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []TagFilter{}, tc.pipeline.LabelFilters)
			require.Equal(t, []TagFilter{}, tc.pipeline.AttrFilters)
			require.Equal(t, []ProcessorConfig{}, tc.pipeline.ProcessorConfigs)
			require.Equal(t, []WorkflowNode{}, tc.pipeline.Nodes)
			require.Equal(t, Connections{}, tc.pipeline.Connections)
			require.Equal(t, []InputVariable{}, tc.pipeline.Inputs)
		})
	}
}

func TestEventPipelineWorkflow_ConvertsLegacyProcessorsToLinearNodes(t *testing.T) {
	pipeline := EventPipeline{ProcessorConfigs: []ProcessorConfig{
		{Typ: "first", Config: map[string]interface{}{"key": "value"}},
		{Typ: "second", Config: "configuration"},
	}}

	pipeline.FillWorkflowFields()

	require.Equal(t, []WorkflowNode{
		{ID: "node_0", Name: "first", Type: "first", Config: map[string]interface{}{"key": "value"}},
		{ID: "node_1", Name: "second", Type: "second", Config: "configuration"},
	}, pipeline.Nodes)
	require.Equal(t, Connections{"node_0": {Main: [][]ConnectionTarget{{{Node: "node_1", Type: "main", Index: 0}}}}},
		pipeline.Connections)
	require.NotContains(t, pipeline.Connections, "node_1")
}

func TestEventPipelineWorkflow_PreservesExplicitGraphAndInputValues(t *testing.T) {
	pipeline := EventPipeline{
		Nodes:            []WorkflowNode{{ID: "custom", Type: "custom"}},
		Connections:      Connections{"custom": {Main: [][]ConnectionTarget{{{Node: "custom", Type: "main"}}}}},
		ProcessorConfigs: []ProcessorConfig{{Typ: "legacy"}},
		Inputs:           []InputVariable{{Key: "env", Value: "prod"}, {Key: "team", Value: "ops"}},
	}

	pipeline.FillWorkflowFields()

	require.Equal(t, pipeline.Nodes, pipeline.GetWorkflowNodes())
	require.Equal(t, "custom", pipeline.Nodes[0].ID)
	require.Equal(t, pipeline.Connections, pipeline.GetWorkflowConnections())
	require.Equal(t, map[string]string{"env": "prod", "team": "ops"}, pipeline.GetInputsMap())
}

func TestEventPipelineList_FiltersGroupAndUseCaseAndNormalizesArrays(t *testing.T) {
	c := pipelineContext(t, &EventPipeline{})
	for _, pipeline := range []EventPipeline{
		{ID: 1, Name: "zeta", GroupId: 7, UseCase: UseCaseEventPipeline},
		{ID: 2, Name: "alpha", GroupId: 7, UseCase: UseCaseAlertRule},
		{ID: 3, Name: "beta", GroupId: 0, TeamIds: []int64{8}, UseCase: UseCaseEventPipeline},
	} {
		pipeline := pipeline
		require.NoError(t, CreateEventPipeline(c, &pipeline))
	}

	pipelines, err := ListEventPipelinesByUseCase(c, 7, UseCaseEventPipeline)

	require.NoError(t, err)
	require.Len(t, pipelines, 1)
	require.Equal(t, int64(1), pipelines[0].ID)
	require.Equal(t, []WorkflowNode{}, pipelines[0].Nodes)
	pipelines, err = ListEventPipelines(c, 0)
	require.NoError(t, err)
	require.Len(t, pipelines, 1)
	require.Equal(t, int64(3), pipelines[0].ID)
	pipelines, err = ListEventPipelines(c)
	require.NoError(t, err)
	require.Len(t, pipelines, 3)
	require.Equal(t, "alpha", pipelines[0].Name)
	selected, err := GetEventPipelinesByIds(c, []int64{1, 99})
	require.NoError(t, err)
	require.Len(t, selected, 1)
	require.Equal(t, int64(1), selected[0].ID)
}

func TestEventPipelineUpdate_PreservesIdentityAndCreationMetadata(t *testing.T) {
	c := pipelineContext(t, &EventPipeline{})
	original := EventPipeline{ID: 1, Name: "old", GroupId: 7, Disabled: true, CreateAt: 100, CreateBy: "author"}
	require.NoError(t, CreateEventPipeline(c, &original))
	replacement := EventPipeline{ID: 99, Name: "new", GroupId: 7, CreateAt: 999, CreateBy: "untrusted", UpdateBy: "editor"}

	require.NoError(t, original.Update(c, &replacement))
	stored, err := GetEventPipeline(c, 1)

	require.NoError(t, err)
	require.Equal(t, "new", stored.Name)
	require.Equal(t, int64(100), stored.CreateAt)
	require.Equal(t, "author", stored.CreateBy)
	require.Equal(t, "editor", stored.UpdateBy)
	require.False(t, stored.Disabled)
	require.Equal(t, []WorkflowNode{}, stored.Nodes)
}

func TestEventPipelineDisable_PreservesConfigurationAndUpdatesAuditFields(t *testing.T) {
	c := pipelineContext(t, &EventPipeline{})
	original := EventPipeline{
		ID: 1, Name: "pipeline", GroupId: 7, UpdateAt: 1,
		ProcessorConfigs: []ProcessorConfig{{Typ: "custom", Config: map[string]interface{}{"key": "value"}}},
	}
	require.NoError(t, CreateEventPipeline(c, &original))
	require.NoError(t, CreateEventPipeline(c, &EventPipeline{ID: 2, Name: "other", GroupId: 8, UpdateAt: 1}))

	require.NoError(t, UpdateEventPipelinesDisabled(c, nil, true, "ignored"))
	require.NoError(t, UpdateEventPipelinesDisabled(c, []int64{1}, true, "operator"))
	stored, err := GetEventPipeline(c, 1)

	require.NoError(t, err)
	require.True(t, stored.Disabled)
	require.Equal(t, "operator", stored.UpdateBy)
	require.Equal(t, original.ProcessorConfigs, stored.ProcessorConfigs)
	stats, err := EventPipelineStatistics(c)
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Total)
	require.Equal(t, stored.UpdateAt, stats.LastUpdated)
	other, err := GetEventPipeline(c, 2)
	require.NoError(t, err)
	require.False(t, other.Disabled)
	require.Equal(t, int64(1), other.UpdateAt)
	require.NoError(t, UpdateEventPipelinesDisabled(c, []int64{1}, false, "operator"))
	stored, err = GetEventPipeline(c, 1)
	require.NoError(t, err)
	require.False(t, stored.Disabled)
}

func TestEventPipelineFillTeamNames_PreservesOrderAndSkipsDeletedTeams(t *testing.T) {
	c := pipelineContext(t, &UserGroup{})
	require.NoError(t, c.DB.Create(&[]UserGroup{{Id: 7, Name: "ops"}, {Id: 8, Name: "platform"}}).Error)
	pipeline := EventPipeline{TeamIds: []int64{8, 99, 7}, TeamNames: []string{"stale"}}

	require.NoError(t, pipeline.FillTeamNames(c))

	require.Equal(t, []string{"platform", "ops"}, pipeline.TeamNames)
	pipeline.TeamIds = nil
	require.NoError(t, pipeline.FillTeamNames(c))
	require.Equal(t, []string{}, pipeline.TeamNames)
}

func TestEventPipelineDeletion_DeletesOnlyRequestedIDs(t *testing.T) {
	c := pipelineContext(t, &EventPipeline{})
	for _, id := range []int64{1, 2, 3} {
		require.NoError(t, CreateEventPipeline(c, &EventPipeline{ID: id, Name: "pipeline", GroupId: 7}))
	}

	require.NoError(t, DeleteEventPipeline(c, 1))
	require.NoError(t, DeleteEventPipelines(c, []int64{2, 99}))

	pipelines, err := ListEventPipelines(c)
	require.NoError(t, err)
	require.Len(t, pipelines, 1)
	require.Equal(t, int64(3), pipelines[0].ID)
}

type round2Processor struct {
	value string
}

func (p *round2Processor) Init(settings interface{}) (Processor, error) {
	value, ok := settings.(string)
	if !ok {
		return nil, fmt.Errorf("settings must be a string")
	}
	return &round2Processor{value: p.value + value}, nil
}

func (p *round2Processor) Process(_ *ctx.Context, workflow *WorkflowContext) (*WorkflowContext, string, error) {
	return workflow, p.value, nil
}

func TestProcessorRegistry_PreservesFirstRegistrationAndInitializesSettings(t *testing.T) {
	const typ = "round2-test-processor"
	previous, existed := processorRegister[typ]
	t.Cleanup(func() {
		if existed {
			processorRegister[typ] = previous
		} else {
			delete(processorRegister, typ)
		}
	})
	RegisterProcessor(typ, &round2Processor{value: "first:"})
	RegisterProcessor(typ, &round2Processor{value: "second:"})

	processor, err := GetProcessorByType("  "+typ+"  ", "configured")

	require.NoError(t, err)
	workflow := &WorkflowContext{}
	result, message, err := processor.Process(nil, workflow)
	require.NoError(t, err)
	require.Same(t, workflow, result)
	require.Equal(t, "first:configured", message)
	_, err = GetProcessorByType(typ, 42)
	require.EqualError(t, err, "settings must be a string")
	_, err = GetProcessorByType("round2-unknown", nil)
	require.EqualError(t, err, "processor type round2-unknown not found")
}
