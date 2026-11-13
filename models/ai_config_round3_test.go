package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAILLMConfigVerify_RequiresProviderConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		change  func(*AILLMConfig)
		message string
	}{
		{name: "name", change: func(v *AILLMConfig) { v.Name = " " }, message: "name is required"},
		{name: "type", change: func(v *AILLMConfig) { v.APIType = "" }, message: "api_type is required"},
		{name: "url", change: func(v *AILLMConfig) { v.APIURL = "" }, message: "api_url is required"},
		{name: "key", change: func(v *AILLMConfig) { v.APIKey = "" }, message: "api_key is required"},
		{name: "model", change: func(v *AILLMConfig) { v.Model = "" }, message: "model is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := AILLMConfig{Name: "provider", APIType: "openai", APIURL: "/v1", APIKey: "secret", Model: "model"}
			tc.change(&config)

			err := config.Verify()

			require.EqualError(t, err, tc.message)
		})
	}
	config := AILLMConfig{Name: " provider ", APIType: "openai", APIURL: "/v1", APIKey: "secret", Model: "model"}
	require.NoError(t, config.Verify())
	require.Equal(t, "provider", config.Name)
}

func TestAILLMConfigMaskAPIKey_RecognizesOnlyTheStoredKeysMask(t *testing.T) {
	cases := []struct {
		name   string
		key    string
		masked string
	}{
		{name: "empty", key: "", masked: ""},
		{name: "short", key: "abc", masked: "****"},
		{name: "boundary", key: "12345678", masked: "****"},
		{name: "long", key: "123456789", masked: "1234****6789"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := AILLMConfig{APIKey: tc.key}

			config.MaskAPIKey()

			require.Equal(t, tc.masked, config.APIKey)
			require.Equal(t, tc.key != "", IsMaskedAPIKey(config.APIKey, tc.key))
			require.False(t, IsMaskedAPIKey("replacement", tc.key))
			require.False(t, IsMaskedAPIKey("", tc.key))
		})
	}
}

func TestAILLMConfigCreate_TransfersDefaultAndRejectsDuplicateNames(t *testing.T) {
	c := pipelineContext(t, &AILLMConfig{})
	first := AILLMConfig{Name: "first", Enabled: true}
	require.NoError(t, first.Create(c, "alice"))
	require.True(t, first.IsDefault)
	second := AILLMConfig{Name: "second", Enabled: true, IsDefault: true}

	require.NoError(t, second.Create(c, "bob"))
	duplicate := AILLMConfig{Name: "second", IsDefault: true}
	require.ErrorContains(t, duplicate.Create(c, "eve"), "already exists")

	got, err := AILLMConfigGetByName(c, "first")
	require.NoError(t, err)
	require.False(t, got.IsDefault)
	require.Equal(t, "alice", got.CreatedBy)
	require.Equal(t, got.CreatedAt, got.UpdatedAt)
	selected, err := AILLMConfigPickDefault(c)
	require.NoError(t, err)
	require.Equal(t, second.Id, selected.Id)
	all, err := AILLMConfigGets(c)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Equal(t, first.Id, all[0].Id)
	require.Equal(t, second.Id, all[1].Id)
}

func TestAILLMConfigUpdate_PreservesCreationAuditAndWritesZeroValues(t *testing.T) {
	c := pipelineContext(t, &AILLMConfig{})
	first := AILLMConfig{
		Id: 1, Name: "first", Enabled: true, IsDefault: true, CreatedAt: 10, CreatedBy: "alice",
		Description: "old description", APIKey: "old key",
	}
	second := AILLMConfig{Id: 2, Name: "second", Enabled: true, Description: "old", APIKey: "old"}
	require.NoError(t, c.DB.Create(&first).Error)
	require.NoError(t, c.DB.Create(&second).Error)
	data := AILLMConfig{
		Name: "renamed", IsDefault: true, CreatedAt: 999, CreatedBy: "eve",
		ExtraConfig: LLMExtraConfig{CustomHeaders: map[string]string{"X-Team": "ops"}},
	}

	require.NoError(t, first.Update(c, "bob", data))

	got, err := AILLMConfigGetById(c, first.Id)
	require.NoError(t, err)
	require.Equal(t, "renamed", got.Name)
	require.Equal(t, int64(10), got.CreatedAt)
	require.Equal(t, "alice", got.CreatedBy)
	require.Equal(t, "bob", got.UpdatedBy)
	require.False(t, got.Enabled)
	require.Empty(t, got.APIKey)
	require.Empty(t, got.Description)
	require.Equal(t, map[string]string{"X-Team": "ops"}, got.ExtraConfig.CustomHeaders)
	// A disabled default must not be selected for runtime use.
	selected, err := AILLMConfigPickDefault(c)
	require.NoError(t, err)
	require.Nil(t, selected)
	enabled, err := AILLMConfigGetEnabled(c)
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	require.Equal(t, second.Id, enabled[0].Id)
}

func TestAILLMConfigUpdate_RejectsConflictingNameWithoutChangingDefault(t *testing.T) {
	c := pipelineContext(t, &AILLMConfig{})
	rows := []AILLMConfig{{Id: 1, Name: "first", IsDefault: true}, {Id: 2, Name: "second"}}
	require.NoError(t, c.DB.Create(&rows).Error)

	err := rows[1].Update(c, "bob", AILLMConfig{Name: "first", IsDefault: true})

	require.ErrorContains(t, err, "already exists")
	got, err := AILLMConfigGetById(c, 1)
	require.NoError(t, err)
	require.True(t, got.IsDefault)
	got, err = AILLMConfigGetById(c, 2)
	require.NoError(t, err)
	require.Equal(t, "second", got.Name)
	require.False(t, got.IsDefault)
}

func TestAILLMConfigGetByIds_SelectsOnlyRequestedConfigs(t *testing.T) {
	c := pipelineContext(t, &AILLMConfig{})
	rows := []AILLMConfig{{Id: 1, Name: "first"}, {Id: 2, Name: "second"}}
	require.NoError(t, c.DB.Create(&rows).Error)

	got, err := AILLMConfigGetByIds(c, []int64{2, 99})

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].Id)
	empty, err := AILLMConfigGetByIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestAILLMConfigDelete_RemovesOnlySelectedConfig(t *testing.T) {
	c := pipelineContext(t, &AILLMConfig{})
	rows := []AILLMConfig{{Id: 1, Name: "first"}, {Id: 2, Name: "second"}}
	require.NoError(t, c.DB.Create(&rows).Error)

	require.NoError(t, rows[0].Delete(c))

	missing, err := AILLMConfigGetById(c, 1)
	require.NoError(t, err)
	require.Nil(t, missing)
	remaining, err := AILLMConfigGets(c)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	require.Equal(t, int64(2), remaining[0].Id)
}

func TestAIAgentVerify_RequiresTrimmedNameAndPositiveConfigID(t *testing.T) {
	cases := []struct {
		name    string
		agent   AIAgent
		message string
	}{
		{name: "blank name", agent: AIAgent{Name: " ", LLMConfigId: 1}, message: "name is required"},
		{name: "zero config", agent: AIAgent{Name: "chat"}, message: "llm_config_id is required"},
		{name: "negative config", agent: AIAgent{Name: "chat", LLMConfigId: -1}, message: "llm_config_id is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.agent.Verify()

			require.EqualError(t, err, tc.message)
		})
	}
	agent := AIAgent{Name: " chat ", LLMConfigId: 1}
	require.NoError(t, agent.Verify())
	require.Equal(t, "chat", agent.Name)
}

func TestAIAgentUpdate_PreservesAuditAndClearsOptionalFields(t *testing.T) {
	c := pipelineContext(t, &AIAgent{})
	agent := AIAgent{Name: "chat", UseCase: "chat", Enabled: true, SkillIds: []int64{1}, MCPServerIds: []int64{2}}
	require.NoError(t, agent.Create(c, "alice"))
	createdAt := agent.CreatedAt

	require.NoError(t, agent.Update(c, "bob", AIAgent{Name: "renamed", LLMConfigId: 3}))

	got, err := AIAgentGetById(c, agent.Id)
	require.NoError(t, err)
	require.Equal(t, "renamed", got.Name)
	require.Equal(t, int64(3), got.LLMConfigId)
	require.False(t, got.Enabled)
	require.Empty(t, got.SkillIds)
	require.Empty(t, got.MCPServerIds)
	require.Equal(t, "alice", got.CreatedBy)
	require.Equal(t, createdAt, got.CreatedAt)
	require.Equal(t, "bob", got.UpdatedBy)
}

func TestAIAgentWrites_RejectDuplicateNames(t *testing.T) {
	c := pipelineContext(t, &AIAgent{})
	first := AIAgent{Name: "first"}
	second := AIAgent{Name: "second"}
	require.NoError(t, first.Create(c, "alice"))
	require.NoError(t, second.Create(c, "alice"))
	duplicate := AIAgent{Name: "first"}

	require.ErrorContains(t, duplicate.Create(c, "bob"), "already exists")
	require.ErrorContains(t, second.Update(c, "bob", AIAgent{Name: "first"}), "already exists")

	got, err := AIAgentGetByName(c, "second")
	require.NoError(t, err)
	require.Equal(t, second.Id, got.Id)
}

func TestAIAgentGetByUseCase_SelectsOnlyEnabledMatchingAgent(t *testing.T) {
	c := pipelineContext(t, &AIAgent{})
	rows := []AIAgent{
		{Id: 1, Name: "disabled", UseCase: "chat"},
		{Id: 2, Name: "enabled", UseCase: "chat", Enabled: true, SkillIds: []int64{3, 4}},
		{Id: 3, Name: "other", UseCase: "diagnosis", Enabled: true},
	}
	require.NoError(t, c.DB.Create(&rows).Error)

	got, err := AIAgentGetByUseCase(c, "chat")

	require.NoError(t, err)
	require.Equal(t, int64(2), got.Id)
	require.Equal(t, []int64{3, 4}, got.SkillIds)
	missing, err := AIAgentGetByUseCase(c, "missing")
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestAIAgentDelete_RemovesOnlySelectedAgent(t *testing.T) {
	c := pipelineContext(t, &AIAgent{})
	rows := []AIAgent{{Id: 1, Name: "first"}, {Id: 2, Name: "second"}, {Id: 3, Name: "third"}}
	require.NoError(t, c.DB.Create(&rows).Error)

	require.NoError(t, rows[1].Delete(c))

	all, err := AIAgentGets(c)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Equal(t, int64(1), all[0].Id)
	require.Equal(t, int64(3), all[1].Id)
}

func TestInitDefaultAIAgent_SeedsOnlyAnEmptyTable(t *testing.T) {
	c := pipelineContext(t, &AIAgent{})

	InitDefaultAIAgent(c)
	InitDefaultAIAgent(c)

	rows, err := AIAgentGets(c)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "default-chat-agent", rows[0].Name)
	require.Equal(t, "chat", rows[0].UseCase)
	require.Equal(t, int64(0), rows[0].LLMConfigId)
	require.True(t, rows[0].Enabled)
	require.Equal(t, "system", rows[0].CreatedBy)
	require.Equal(t, rows[0].CreatedAt, rows[0].UpdatedAt)
}
