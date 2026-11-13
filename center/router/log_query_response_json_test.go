package router

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/gin-gonic/gin"
)

func TestLogQueryRowEncodingFailureEnvelope(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial", true: "after output"}[started], func(t *testing.T) {
			engine := gin.New()
			engine.Use(aop.Recovery())
			engine.GET("/", func(c *gin.Context) {
				w := NewLogQueryResponse(c)
				if started {
					if err := w.Row(map[string]any{"body": strings.Repeat("x", 64<<10)}); err != nil {
						t.Fatal(err)
					}
				}
				err := w.Row(map[string]any{"a": strings.Repeat("bad row", 10000), "z": math.NaN()})
				if err == nil {
					t.Fatal("NaN accepted")
				}
				w.Fail(err)
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
			var body struct {
				Err string `json:"err"`
				Dat *struct {
					List  []map[string]any `json:"list"`
					Total int64            `json:"total"`
				} `json:"dat"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if response.Code != 200 || !strings.Contains(body.Err, "unsupported value: NaN") {
				t.Fatalf("status=%d err=%q", response.Code, body.Err)
			}
			if started && (body.Dat == nil || body.Dat.Total != 0 || len(body.Dat.List) != 1 || body.Dat.List[0]["body"] != strings.Repeat("x", 64<<10)) {
				t.Fatalf("failure changed successful rows: %+v", body.Dat)
			}
			if !started && body.Dat != nil {
				t.Fatalf("initial encoding failure returned rows: %+v", body.Dat)
			}
		})
	}
}

type jsonOutputSink struct {
	gin.ResponseWriter
	max, total int
	fail       error
	short      bool
}

func (w *jsonOutputSink) WriteString(s string) (int, error) {
	if len(s) > w.max {
		w.max = len(s)
	}
	if w.fail != nil {
		return 0, w.fail
	}
	if w.short {
		return len(s) - 1, nil
	}
	w.total += len(s)
	return len(s), nil
}
func TestLogQueryOutputBoundedCopy(t *testing.T) {
	sink := &jsonOutputSink{}
	body := make([]byte, 50<<20)
	n, err := (logQueryStreamingBody{sink}).Write(body)
	if err != nil || n != len(body) || sink.total != len(body) || sink.max > 32<<10 {
		t.Fatalf("n=%d total=%d max=%d err=%v", n, sink.total, sink.max, err)
	}
	for _, tc := range []struct {
		sink *jsonOutputSink
		err  error
	}{{&jsonOutputSink{fail: io.ErrClosedPipe}, io.ErrClosedPipe}, {&jsonOutputSink{short: true}, io.ErrShortWrite}} {
		if _, err := (logQueryStreamingBody{tc.sink}).Write(body); !errors.Is(err, tc.err) {
			t.Fatalf("got %v want %v", err, tc.err)
		}
	}
}

type cancelJSONRecorder struct {
	*httptest.ResponseRecorder
	cancel func()
}

func (w *cancelJSONRecorder) WriteString(s string) (int, error) {
	n, err := w.ResponseRecorder.WriteString(s)
	w.cancel()
	return n, err
}
func TestLogQueryCancellationCompletesCurrentRow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.GET("/", func(c *gin.Context) {
		w := NewLogQueryResponse(c)
		err := w.Row(map[string]any{"body": strings.Repeat("x", 128<<10)})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("row cancellation=%v", err)
		}
		w.Fail(err)
	})
	rec := &cancelJSONRecorder{httptest.NewRecorder(), cancel}
	engine.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil).WithContext(ctx))
	var body struct {
		Dat struct {
			List  []map[string]string `json:"list"`
			Total int64               `json:"total"`
		} `json:"dat"`
		Err string `json:"err"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("canceled response invalid: %v", err)
	}
	if body.Err != "context canceled" || body.Dat.Total != 0 || len(body.Dat.List) != 1 || body.Dat.List[0]["body"] != strings.Repeat("x", 128<<10) {
		t.Fatalf("canceled envelope: rows=%d total=%d err=%q", len(body.Dat.List), body.Dat.Total, body.Err)
	}
}
