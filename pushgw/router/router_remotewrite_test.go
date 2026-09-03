package router

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func genUniqueLabels(n int) []prompb.Label {
	ls := make([]prompb.Label, n)
	for i := 0; i < n; i++ {
		ls[i] = prompb.Label{Name: "l" + itoa(i), Value: "v"}
	}
	return ls
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func TestDuplicateLabelKey(t *testing.T) {
	tests := []struct {
		name   string
		labels []prompb.Label
		want   bool
	}{
		{"nil", nil, false},
		{"empty", []prompb.Label{}, false},
		{"unique", []prompb.Label{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}}, false},
		{"dup-head", []prompb.Label{{Name: "a", Value: "1"}, {Name: "a", Value: "2"}}, true},
		{"dup-tail", []prompb.Label{
			{Name: "__name__", Value: "x"},
			{Name: "ident", Value: "h1"},
			{Name: "job", Value: "n9e"},
			{Name: "job", Value: "n9e2"},
		}, true},
		{"many-unique", []prompb.Label{
			{Name: "__name__", Value: "x"}, {Name: "a", Value: "1"}, {Name: "b", Value: "2"},
			{Name: "c", Value: "3"}, {Name: "d", Value: "4"}, {Name: "e", Value: "5"},
			{Name: "f", Value: "6"}, {Name: "g", Value: "7"}, {Name: "h", Value: "8"},
		}, false},
		{"large-unique-triggers-map-path", genUniqueLabels(duplicateLabelKeyLinearThreshold + 16), false},
		{"large-dup-triggers-map-path", func() []prompb.Label {
			ls := genUniqueLabels(duplicateLabelKeyLinearThreshold + 16)
			ls = append(ls, prompb.Label{Name: ls[3].Name, Value: "dup"})
			return ls
		}(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := &prompb.TimeSeries{Labels: tt.labels}
			if got := duplicateLabelKey(ts); got != tt.want {
				t.Fatalf("duplicateLabelKey=%v, want %v", got, tt.want)
			}
		})
	}

	if duplicateLabelKey(nil) {
		t.Fatalf("duplicateLabelKey(nil) must be false")
	}
}

func TestExtractIdentFromTimeSeries(t *testing.T) {
	type args struct {
		labels       []prompb.Label
		ignoreIdent  bool
		ignoreHost   bool
		identMetrics []string
	}
	tests := []struct {
		name        string
		in          args
		wantIdent   string
		wantInsert  bool
		wantLabel0  string // 首个 label 名字，用于校验 agent_hostname/host 是否被重命名为 ident
		wantLabel0v string
	}{
		{
			name:       "explicit ident wins",
			in:         args{labels: []prompb.Label{{Name: "ident", Value: "host-a"}, {Name: "host", Value: "domain"}}, ignoreHost: false},
			wantIdent:  "host-a",
			wantInsert: true,
			wantLabel0: "ident", wantLabel0v: "host-a",
		},
		{
			name:       "fallback to agent_hostname and rewrite label name",
			in:         args{labels: []prompb.Label{{Name: "agent_hostname", Value: "agent-x"}}, ignoreHost: true},
			wantIdent:  "agent-x",
			wantInsert: true,
			wantLabel0: "ident", wantLabel0v: "agent-x",
		},
		{
			name:       "fallback to host when !ignoreHost",
			in:         args{labels: []prompb.Label{{Name: "host", Value: "h-1"}}, ignoreHost: false},
			wantIdent:  "h-1",
			wantInsert: true,
			wantLabel0: "ident", wantLabel0v: "h-1",
		},
		{
			name:       "ignoreHost drops host fallback",
			in:         args{labels: []prompb.Label{{Name: "host", Value: "h-1"}}, ignoreHost: true},
			wantIdent:  "",
			wantInsert: false,
			wantLabel0: "host", wantLabel0v: "h-1",
		},
		{
			name:       "no ident-bearing label",
			in:         args{labels: []prompb.Label{{Name: "foo", Value: "bar"}}, ignoreHost: false},
			wantIdent:  "",
			wantInsert: false,
			wantLabel0: "foo", wantLabel0v: "bar",
		},
		{
			name: "identMetrics matched",
			in: args{
				labels:       []prompb.Label{{Name: "__name__", Value: "cpu_util"}, {Name: "ident", Value: "h1"}},
				identMetrics: []string{"mem_util", "cpu_util"},
			},
			wantIdent:  "h1",
			wantInsert: true,
		},
		{
			name: "identMetrics unmatched => insert=false but ident returned",
			in: args{
				labels:       []prompb.Label{{Name: "__name__", Value: "disk_used"}, {Name: "ident", Value: "h1"}},
				identMetrics: []string{"cpu_util"},
			},
			wantIdent:  "h1",
			wantInsert: false,
		},
		{
			name: "ignoreIdent => insert=false",
			in: args{
				labels:      []prompb.Label{{Name: "ident", Value: "h1"}},
				ignoreIdent: true,
			},
			wantIdent:  "h1",
			wantInsert: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := &prompb.TimeSeries{Labels: append([]prompb.Label(nil), tt.in.labels...)}
			ident, insert := extractIdentFromTimeSeries(ts, tt.in.ignoreIdent, tt.in.ignoreHost, tt.in.identMetrics)
			if ident != tt.wantIdent || insert != tt.wantInsert {
				t.Fatalf("got=(%q,%v) want=(%q,%v)", ident, insert, tt.wantIdent, tt.wantInsert)
			}
			if tt.wantLabel0 != "" {
				if ts.Labels[0].Name != tt.wantLabel0 || ts.Labels[0].Value != tt.wantLabel0v {
					t.Fatalf("labels[0]=%+v want name=%s value=%s", ts.Labels[0], tt.wantLabel0, tt.wantLabel0v)
				}
			}
		})
	}
}

func TestDecodeWriteRequestRoundTrip(t *testing.T) {
	orig := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels:  []prompb.Label{{Name: "__name__", Value: "up"}, {Name: "ident", Value: "h1"}},
				Samples: []prompb.Sample{{Value: 1, Timestamp: 1000}},
			},
			{
				Labels:  []prompb.Label{{Name: "__name__", Value: "cpu_util"}, {Name: "ident", Value: "h2"}},
				Samples: []prompb.Sample{{Value: 0.42, Timestamp: 2000}, {Value: 0.58, Timestamp: 3000}},
			},
		},
	}
	raw, err := proto.Marshal(orig)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	compressed := snappy.Encode(nil, raw)

	// 多次调用以确保池中缓冲的复用路径正确
	for i := 0; i < 5; i++ {
		req, err := DecodeWriteRequest(io.NopCloser(bytes.NewReader(compressed)))
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if !reflect.DeepEqual(req.Timeseries, orig.Timeseries) {
			t.Fatalf("iter %d: mismatch: %+v", i, req.Timeseries)
		}
	}
}

func TestDecodeWriteRequestLargePayload(t *testing.T) {
	// 构造一个会把 body 缓冲撑大但不超过回收阈值的负载
	orig := &prompb.WriteRequest{}
	for i := 0; i < 2000; i++ {
		orig.Timeseries = append(orig.Timeseries, prompb.TimeSeries{
			Labels:  []prompb.Label{{Name: "__name__", Value: "m"}, {Name: "i", Value: "v"}},
			Samples: []prompb.Sample{{Value: float64(i), Timestamp: int64(i)}},
		})
	}
	raw, _ := proto.Marshal(orig)
	compressed := snappy.Encode(nil, raw)

	req, err := DecodeWriteRequest(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(req.Timeseries) != len(orig.Timeseries) {
		t.Fatalf("len=%d want=%d", len(req.Timeseries), len(orig.Timeseries))
	}
}
