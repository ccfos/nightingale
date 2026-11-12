package models

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func emptyNotifyDirectory(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "n9e-notify-round5-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Remove(dir)) })
	return dir
}

func TestInitNotifyConfig_SeedsDefaultChannelsAndTemplatesIdempotently(t *testing.T) {
	c := pipelineContext(t, &Configs{}, &NotifyTpl{})
	dir := emptyNotifyDirectory(t)

	InitNotifyConfig(c, dir)
	InitNotifyConfig(c, dir)

	value, err := ConfigsGet(c, NOTIFYCHANNEL)
	require.NoError(t, err)
	var channels []NotifyChannel
	require.NoError(t, json.Unmarshal([]byte(value), &channels))
	require.Len(t, channels, len(DefaultChannels))
	for i, channel := range channels {
		require.Equal(t, DefaultChannels[i], channel.Ident)
		require.True(t, channel.BuiltIn)
	}
	templates, err := NotifyTplGets(c)
	require.NoError(t, err)
	require.Len(t, templates, len(TplMap))
	for _, tpl := range templates {
		require.Equal(t, TplMap[tpl.Channel], tpl.Content)
	}
}

func TestInitNotifyConfig_MergesMissingDefaultsAndPreservesCustomTemplates(t *testing.T) {
	c := pipelineContext(t, &Configs{}, &NotifyTpl{})
	dir := emptyNotifyDirectory(t)
	initial := []NotifyChannel{{Ident: "custom", Name: "Custom"}, {Ident: DefaultChannels[0], Name: "Renamed"}}
	data, err := json.Marshal(initial)
	require.NoError(t, err)
	require.NoError(t, ConfigsSet(c, NOTIFYCHANNEL, string(data)))
	original := NotifyTpl{Channel: DefaultChannels[0], Name: "Customized", Content: "retained"}
	require.NoError(t, original.Create(c))

	InitNotifyConfig(c, dir)

	value, err := ConfigsGet(c, NOTIFYCHANNEL)
	require.NoError(t, err)
	var channels []NotifyChannel
	require.NoError(t, json.Unmarshal([]byte(value), &channels))
	require.Len(t, channels, len(DefaultChannels)+1)
	require.Equal(t, initial, channels[:2])
	got, err := NotifyTplGet(c, original.Id)
	require.NoError(t, err)
	require.Equal(t, "retained", got.Content)
	require.Equal(t, "Customized", got.Name)
}

func TestInitNotifyConfig_InvalidChannelJSONLeavesTemplatesUnchanged(t *testing.T) {
	c := pipelineContext(t, &Configs{}, &NotifyTpl{})
	require.NoError(t, ConfigsSet(c, NOTIFYCHANNEL, "{"))

	InitNotifyConfig(c, emptyNotifyDirectory(t))

	templates, err := NotifyTplGets(c)
	require.NoError(t, err)
	require.Empty(t, templates)
	value, err := ConfigsGet(c, NOTIFYCHANNEL)
	require.NoError(t, err)
	require.Equal(t, "{", value)
}
