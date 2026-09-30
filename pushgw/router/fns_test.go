package router

import (
	"reflect"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/prometheus/prometheus/prompb"
)

func TestAppendLabelsTargetTagControls(t *testing.T) {
	tests := []struct {
		name           string
		appendTags     bool
		appendHostTags bool
		want           map[string]string
	}{
		{
			name:           "both enabled preserves host tag precedence",
			appendTags:     true,
			appendHostTags: true,
			want: map[string]string{
				"__name__":  "test_metric",
				"custom":    "custom-value",
				"reported":  "reported-value",
				"duplicate": "reported-value",
			},
		},
		{
			name:       "only custom target tags enabled",
			appendTags: true,
			want: map[string]string{
				"__name__":  "test_metric",
				"custom":    "custom-value",
				"duplicate": "custom-value",
			},
		},
		{
			name:           "only reported host tags enabled",
			appendHostTags: true,
			want: map[string]string{
				"__name__":  "test_metric",
				"reported":  "reported-value",
				"duplicate": "reported-value",
			},
		},
		{
			name: "both disabled",
			want: map[string]string{
				"__name__": "test_metric",
			},
		},
	}

	target := &models.Target{
		Tags:     "custom=custom-value duplicate=custom-value ",
		HostTags: []string{"reported=reported-value", "duplicate=reported-value"},
	}
	target.FillTagsMap()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &Router{Pushgw: pconf.Pushgw{
				EnableTargetTagAppend:     tt.appendTags,
				EnableTargetHostTagAppend: tt.appendHostTags,
				LabelRewrite:              true,
			}}
			series := &prompb.TimeSeries{Labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric"},
			}}

			rt.AppendLabels(series, target, nil)

			if got := seriesLabelsMap(series); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("unexpected labels: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppendLabelsRewriteBehavior(t *testing.T) {
	target := &models.Target{
		Tags:     "duplicate=custom-value custom=custom-value ",
		HostTags: []string{"duplicate=reported-value"},
	}
	target.FillTagsMap()

	tests := []struct {
		name    string
		rewrite bool
		want    string
	}{
		{name: "preserve incoming label", rewrite: false, want: "series-value"},
		{name: "rewrite incoming label", rewrite: true, want: "reported-value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &Router{Pushgw: pconf.Pushgw{
				EnableTargetTagAppend:     true,
				EnableTargetHostTagAppend: true,
				LabelRewrite:              tt.rewrite,
			}}
			series := &prompb.TimeSeries{Labels: []prompb.Label{
				{Name: "__name__", Value: "test_metric"},
				{Name: "duplicate", Value: "series-value"},
			}}

			rt.AppendLabels(series, target, nil)

			if got := seriesLabelsMap(series)["duplicate"]; got != tt.want {
				t.Fatalf("unexpected duplicate label: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppendLabelsBusinessGroupUnaffected(t *testing.T) {
	bgCache := &memsto.BusiGroupCacheType{}
	bgCache.Set(map[int64]*models.BusiGroup{
		1: {Id: 1, LabelEnable: 1, LabelValue: "production"},
	}, 1, 1)

	rt := &Router{Pushgw: pconf.Pushgw{
		EnableTargetTagAppend:     false,
		EnableTargetHostTagAppend: false,
		BusiGroupLabelKey:         "busigroup",
	}}
	series := &prompb.TimeSeries{Labels: []prompb.Label{
		{Name: "__name__", Value: "test_metric"},
	}}
	target := &models.Target{
		GroupId:  1,
		Tags:     "custom=custom-value ",
		HostTags: []string{"reported=reported-value"},
	}
	target.FillTagsMap()

	rt.AppendLabels(series, target, bgCache)

	want := map[string]string{
		"__name__":  "test_metric",
		"busigroup": "production",
	}
	if got := seriesLabelsMap(series); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected labels: got %v, want %v", got, want)
	}
}

func seriesLabelsMap(series *prompb.TimeSeries) map[string]string {
	labels := make(map[string]string, len(series.Labels))
	for _, label := range series.Labels {
		labels[label.Name] = label.Value
	}
	return labels
}
