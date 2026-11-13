package doris

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"
)

type jsonTestMarshaler string

func (v jsonTestMarshaler) MarshalJSON() ([]byte, error) { return []byte(v), nil }

func TestJSONRowCompatibility(t *testing.T) {
	for _, row := range []map[string]any{
		nil, {},
		{"text": "quote\"\\\n\r\t\b\f\x00\x1f<>&\u2028\u2029中文" + string([]byte{0xff, 0xc0, 0xaf}), "nil": nil, "integer": int64(9007199254740993), "unsigned": uint64(math.MaxUint64), "float": 1e-7, "bool": true},
		{"bytes": []byte{0, 1, 255}, "empty": []byte{}, "date": time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC), "nested": map[string]any{"a": "<x>", "b": []any{nil, true, json.Number("1.250")}}, "raw": json.RawMessage(` { "z":"<>&" } `), "custom": jsonTestMarshaler(`{"x":"<"}`)},
		{"\xff": "invalid key", "\u2028": "key", "a": "", "long": strings.Repeat("x", 1<<20) + "中文"},
	} {
		want, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := PrepareJSONRow(row)
		if err != nil {
			t.Fatal(err)
		}
		var got bytes.Buffer
		if err := prepared.Write(context.Background(), &got); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("JSON differs: got %.200s want %.200s", got.Bytes(), want)
		}
	}
}

func TestJSONRowPreflightErrors(t *testing.T) {
	cycle := map[string]any{}
	cycle["cycle"] = cycle
	for _, value := range []any{math.NaN(), math.Inf(1), make(chan int), func() {}, json.RawMessage("broken"), jsonTestMarshaler("broken"), cycle} {
		row := map[string]any{"a": strings.Repeat("x", 1<<20), "z": value}
		if _, err := PrepareJSONRow(row); err == nil {
			t.Fatalf("accepted %T", value)
		}
	}
}

type jsonChunkSink struct {
	total, max int
	cancel     context.CancelFunc
	fail       error
	short      bool
}

func (w *jsonChunkSink) Write(p []byte) (int, error) {
	if len(p) > w.max {
		w.max = len(p)
	}
	if w.fail != nil {
		return 0, w.fail
	}
	if w.short {
		return len(p) - 1, nil
	}
	w.total += len(p)
	if w.cancel != nil {
		w.cancel()
	}
	return len(p), nil
}
func TestJSONRowBoundedWritesAndCancellation(t *testing.T) {
	p, err := PrepareJSONRow(map[string]any{"body": strings.Repeat("x", 50<<20)})
	if err != nil {
		t.Fatal(err)
	}
	sink := &jsonChunkSink{}
	if err := p.Write(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if sink.max > jsonRowChunkSize || sink.total != 50<<20+11 {
		t.Fatalf("max=%d total=%d", sink.max, sink.total)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	canceled := &jsonChunkSink{cancel: cancel}
	if err := p.Write(ctx, canceled); !errors.Is(err, context.Canceled) || (canceled.total <= 0 || canceled.total > jsonRowChunkSize) {
		t.Fatalf("cancel err=%v total=%d", err, canceled.total)
	}
	for _, tc := range []struct {
		sink *jsonChunkSink
		err  error
	}{{&jsonChunkSink{fail: io.ErrClosedPipe}, io.ErrClosedPipe}, {&jsonChunkSink{short: true}, io.ErrShortWrite}} {
		if err := p.Write(context.Background(), tc.sink); !errors.Is(err, tc.err) {
			t.Fatalf("got %v want %v", err, tc.err)
		}
	}
}

func FuzzJSONRowString(f *testing.F) {
	for _, s := range []string{"", "<>&\u2028\u2029", "\xff\xc0\xaf", "\x00\x01\x1f\n\r\t\"\\", strings.Repeat("a", jsonRowChunkSize-1) + "中文"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		row := map[string]any{s: s}
		want, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		p, err := PrepareJSONRow(row)
		if err != nil {
			t.Fatal(err)
		}
		var got bytes.Buffer
		if err := p.Write(context.Background(), &got); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("JSON mismatch for %q", s)
		}
	})
}
func BenchmarkJSONRowLarge(b *testing.B) {
	row := map[string]any{"body": strings.Repeat("x", 50<<20), "seq": int64(1)}
	for _, name := range []string{"standard", "chunked"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(50 << 20)
			for i := 0; i < b.N; i++ {
				if name == "standard" {
					if err := json.NewEncoder(io.Discard).Encode(row); err != nil {
						b.Fatal(err)
					}
				} else {
					p, err := PrepareJSONRow(row)
					if err != nil {
						b.Fatal(err)
					}
					if err := p.Write(context.Background(), io.Discard); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func BenchmarkJSONRowSmall(b *testing.B) {
	row := map[string]any{"body": strings.Repeat("x", 1024), "seq": int64(1)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p, err := PrepareJSONRow(row)
		if err != nil {
			b.Fatal(err)
		}
		if err := p.Write(context.Background(), io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}
