package dispatch

import (
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestSendCallbacks_AddsHTTPToSchemelessURL(t *testing.T) {
	callback := &round2Callback{}
	d := &Dispatch{
		userCache: &memsto.UserCacheType{},
		CallBacks: map[string]sender.CallBacker{models.DefaultDomain: callback},
	}
	target := NewNotifyTarget()
	target.callbacks["local.test/hook"] = struct{}{}
	event := &models.AlertCurEvent{Hash: "cpu-high"}

	d.SendCallbacks(&models.AlertRule{}, target, event)

	require.Len(t, callback.calls, 1)
	require.Equal(t, "http://local.test/hook", callback.calls[0].CallBackURL)
	require.Equal(t, []*models.AlertCurEvent{event}, callback.calls[0].Events)
}

func TestSendCallbacks_RejectsMalformedURL(t *testing.T) {
	callback := &round2Callback{}
	d := &Dispatch{
		userCache: &memsto.UserCacheType{},
		CallBacks: map[string]sender.CallBacker{models.DefaultDomain: callback},
	}
	target := NewNotifyTarget()
	target.callbacks["http://%zz/hook"] = struct{}{}

	d.SendCallbacks(&models.AlertRule{}, target, &models.AlertCurEvent{})

	require.Empty(t, callback.calls)
}

func TestPipelineApplicable_InvalidAttributeFilterRejectsEvent(t *testing.T) {
	pipeline := &models.EventPipeline{
		FilterEnable: true, AttrFilters: []models.TagFilter{{Key: "rule_name", Func: "=~", Value: "["}},
	}

	applies := PipelineApplicable(pipeline, &models.AlertCurEvent{RuleName: "CPU"})

	require.False(t, applies)
	require.Equal(t, "[", pipeline.AttrFilters[0].Value)
}

func TestNotifyRuleMatchCheck_RejectsBeforeLastMinuteRangeStarts(t *testing.T) {
	instant, err := time.ParseInLocation("2006-01-02 15:04", "2026-11-09 09:00", time.Local)
	require.NoError(t, err)
	config := &models.NotifyConfig{
		Severities: []int{2}, TimeRanges: []models.TimeRanges{{Start: "10:00", End: "23:59", Week: []int{1}}},
	}

	err = NotifyRuleMatchCheck(config, &models.AlertCurEvent{TriggerTime: instant.Unix(), Severity: 2})

	require.ErrorContains(t, err, "time")
}

func TestNotifyRuleMatchCheck_MatchingRangeIgnoresLaterNonmatchingRanges(t *testing.T) {
	instant, err := time.ParseInLocation("2006-01-02 15:04", "2026-11-09 11:00", time.Local)
	require.NoError(t, err)
	config := &models.NotifyConfig{
		Severities: []int{2}, TimeRanges: []models.TimeRanges{
			{Start: "10:00", End: "12:00", Week: []int{1}},
			{Start: "14:00", End: "16:00", Week: []int{2}},
		},
	}

	err = NotifyRuleMatchCheck(config, &models.AlertCurEvent{TriggerTime: instant.Unix(), Severity: 2})

	require.NoError(t, err)
}
