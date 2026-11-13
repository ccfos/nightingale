package models

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotifyTplCreateIfNotExists_PreservesExistingChannelTemplate(t *testing.T) {
	c := pipelineContext(t, &NotifyTpl{})
	original := NotifyTpl{Name: "original", Channel: "custom", Content: "original content"}
	require.NoError(t, original.CreateIfNotExists(c, "custom"))
	replacement := NotifyTpl{Name: "replacement", Channel: "custom", Content: "replacement content"}

	require.NoError(t, replacement.CreateIfNotExists(c, "custom"))
	rows, err := NotifyTplGets(c)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "original", rows[0].Name)
	require.Equal(t, "original content", rows[0].Content)
	count, err := NotifyTplCountByChannel(c, "custom")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestNotifyTplUpdateContent_PreservesNameAndChannel(t *testing.T) {
	c := pipelineContext(t, &NotifyTpl{})
	row := NotifyTpl{Name: "original", Channel: "custom", Content: "old", CreateBy: "alice"}
	require.NoError(t, row.Create(c))
	row.Name = "discarded"
	row.Channel = "discarded"
	row.Content = ""
	row.UpdateBy = "bob"

	require.NoError(t, row.UpdateContent(c))
	got, err := NotifyTplGet(c, row.Id)

	require.NoError(t, err)
	require.Equal(t, "original", got.Name)
	require.Equal(t, "custom", got.Channel)
	require.Empty(t, got.Content)
	require.Equal(t, "alice", got.CreateBy)
	require.Equal(t, "bob", got.UpdateBy)
}

func TestNotifyTplUpdate_ChangesNameWithoutReplacingContent(t *testing.T) {
	c := pipelineContext(t, &NotifyTpl{})
	row := NotifyTpl{Name: "original", Channel: "custom", Content: "retained"}
	require.NoError(t, row.Create(c))
	row.Name = "renamed"
	row.Content = "discarded"

	require.NoError(t, row.Update(c))
	got, err := NotifyTplGet(c, row.Id)

	require.NoError(t, err)
	require.Equal(t, "renamed", got.Name)
	require.Equal(t, "retained", got.Content)
}

func TestNotifyTplDelete_ProtectsDefaultChannels(t *testing.T) {
	c := pipelineContext(t, &NotifyTpl{})
	builtin := NotifyTpl{Channel: DefaultChannels[0], Name: "builtin"}
	custom := NotifyTpl{Channel: "round4-custom", Name: "custom"}
	require.NoError(t, builtin.Create(c))
	require.NoError(t, custom.Create(c))

	require.NoError(t, builtin.NotifyTplDelete(c, builtin.Id))
	require.NoError(t, custom.NotifyTplDelete(c, custom.Id))
	rows, err := NotifyTplGets(c)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, builtin.Id, rows[0].Id)
}

func TestListTpls_RendersLabelAndValueVariables(t *testing.T) {
	c := pipelineContext(t, &NotifyTpl{})
	row := NotifyTpl{Channel: "custom", Content: `{{$labels.host}}={{$value}}: {{.RuleName}}`}
	require.NoError(t, row.Create(c))

	templates, err := ListTpls(c)

	require.NoError(t, err)
	require.Contains(t, templates, "custom")
	var output bytes.Buffer
	require.NoError(t, templates["custom"].Execute(&output, AlertCurEvent{
		TagsMap: map[string]string{"host": "node-a"}, TriggerValue: "42", RuleName: "CPU high",
	}))
	require.Equal(t, "node-a=42: CPU high", output.String())
}

func TestListTpls_RejectsMalformedStoredTemplates(t *testing.T) {
	c := pipelineContext(t, &NotifyTpl{})
	require.NoError(t, (&NotifyTpl{Channel: "broken", Content: "{{if}}"}).Create(c))

	templates, err := ListTpls(c)

	require.ErrorContains(t, err, "failed to parse tpl")
	require.Nil(t, templates)
}
