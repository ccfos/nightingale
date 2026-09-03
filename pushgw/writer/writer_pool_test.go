package writer

import (
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func TestMarshalAndSnappyEncode_RoundTrip(t *testing.T) {
	items := []prompb.TimeSeries{
		{
			Labels:  []prompb.Label{{Name: "__name__", Value: "cpu_util"}, {Name: "ident", Value: "h1"}},
			Samples: []prompb.Sample{{Value: 0.42, Timestamp: 1000}},
		},
		{
			Labels:  []prompb.Label{{Name: "__name__", Value: "mem_used"}, {Name: "ident", Value: "h2"}},
			Samples: []prompb.Sample{{Value: 1024, Timestamp: 2000}, {Value: 2048, Timestamp: 3000}},
		},
	}

	// 预期结果：直接 proto.Marshal + snappy.Encode
	want, err := proto.Marshal(&prompb.WriteRequest{Timeseries: items})
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	// 多次调用以触发池中缓冲复用，确保每次输出都与参考实现一致
	for i := 0; i < 5; i++ {
		encoded, release, err := marshalAndSnappyEncode(items)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}

		// 拷贝一份再释放 —— 模拟 Post 持有 encoded 的生命周期
		got := append([]byte(nil), encoded...)
		release()

		decoded, err := snappy.Decode(nil, got)
		if err != nil {
			t.Fatalf("iter %d: snappy.Decode: %v", i, err)
		}

		if !reflect.DeepEqual(decoded, want) {
			t.Fatalf("iter %d: marshaled bytes mismatch\n got=%x\nwant=%x", i, decoded, want)
		}

		var req prompb.WriteRequest
		if err := proto.Unmarshal(decoded, &req); err != nil {
			t.Fatalf("iter %d: Unmarshal: %v", i, err)
		}
		if !reflect.DeepEqual(req.Timeseries, items) {
			t.Fatalf("iter %d: ts mismatch: %+v", i, req.Timeseries)
		}
	}
}

func TestForceSampleTS(t *testing.T) {
	items := []prompb.TimeSeries{
		{Samples: []prompb.Sample{{Value: 1, Timestamp: 1}}},
		{Samples: nil}, // 空 sample 应当被跳过，不 panic
		{Samples: []prompb.Sample{{Value: 2, Timestamp: 2}, {Value: 3, Timestamp: 3}}},
	}
	forceSampleTS(items)
	if items[0].Samples[0].Timestamp == 1 {
		t.Fatalf("items[0].Samples[0] timestamp not overwritten")
	}
	if items[2].Samples[0].Timestamp == 2 {
		t.Fatalf("items[2].Samples[0] timestamp not overwritten")
	}
	// 仅首条 sample 被改写
	if items[2].Samples[1].Timestamp != 3 {
		t.Fatalf("items[2].Samples[1] timestamp should remain 3, got %d", items[2].Samples[1].Timestamp)
	}
}
