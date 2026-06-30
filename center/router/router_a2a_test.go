package router

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestA2AWriteDeadlineExempt pins down which paths keep the http.Server
// WriteTimeout backstop (false) vs. which get the deadline cleared (true).
// Getting this wrong either cuts off long agent runs / SSE streams mid-response
// or — as in the original incident — lets a misrouted request pin a handler
// goroutine forever, so the boundary is worth a regression test.
func TestA2AWriteDeadlineExempt(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		// Streaming / long-running agent endpoints: deadline must be relaxed.
		{"/a2a/message:send", true},   // full synchronous agent turn, can take minutes
		{"/a2a/message:stream", true}, // SSE
		{"/a2a/tasks/019f1675-7a93:subscribe", true},
		{"/mcp", true},      // Streamable HTTP root multiplexes unary + SSE
		{"/mcp/", true},     // trailing slash
		{"/mcp/anything", true},

		// Fast request/response endpoints: keep the WriteTimeout backstop.
		{"/a2a", false},                         // method-less root — the leak path
		{"/a2a/tasks/019f1675-7a93", false},     // get task
		{"/a2a/tasks", false},                   // list tasks
		{"/a2a/tasks/019f1675-7a93:cancel", false},
		{"/a2a/message:sendfoo", false},         // not an exact streaming method
		{"/a2afoo", false},                      // not an a2a/mcp path at all
		{"/", false},
		{"", false},
	}
	for _, c := range cases {
		if got := a2aWriteDeadlineExempt(c.path); got != c.want {
			t.Errorf("a2aWriteDeadlineExempt(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// TestA2AResponseCapturePreview pins down the head+tail reconstruction the
// "done" log relies on: a body within budget must round-trip byte-for-byte
// (so a unary reply is logged verbatim), while an over-budget body must keep
// its head and tail with an accurate omitted-byte count in between — the part
// that proves a streaming response reached its terminal event.
func TestA2AResponseCapturePreview(t *testing.T) {
	// Feed the body in many small chunks to mimic SSE frame-by-frame writes;
	// preview() must not depend on write boundaries.
	feed := func(body []byte, chunk int) *a2aResponseCapture {
		w := &a2aResponseCapture{}
		for i := 0; i < len(body); i += chunk {
			end := i + chunk
			if end > len(body) {
				end = len(body)
			}
			w.capture(body[i:end])
		}
		return w
	}
	gen := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte('a' + i%26)
		}
		return b
	}

	// Bodies up to the full head+tail budget must reconstruct exactly.
	for _, n := range []int{0, 1, a2aLogRespHeadLimit - 1, a2aLogRespHeadLimit,
		a2aLogRespHeadLimit + 1, a2aLogRespHeadLimit + a2aLogRespTailLimit} {
		body := gen(n)
		got, truncated := feed(body, 7).preview()
		if truncated {
			t.Errorf("n=%d: unexpected truncation", n)
		}
		if got != string(body) {
			t.Errorf("n=%d: preview not byte-exact (len got=%d want=%d)", n, len(got), n)
		}
	}

	// Over budget: head + marker + tail, with the right omitted count, and the
	// real first/last bytes preserved.
	n := a2aLogRespHeadLimit + a2aLogRespTailLimit + 5000
	body := gen(n)
	got, truncated := feed(body, 64).preview()
	if !truncated {
		t.Fatalf("n=%d: expected truncation", n)
	}
	wantMarker := fmt.Sprintf("...<%d bytes omitted>...", 5000)
	if !strings.Contains(got, wantMarker) {
		t.Errorf("missing omitted marker %q in preview", wantMarker)
	}
	if !strings.HasPrefix(got, string(body[:a2aLogRespHeadLimit])) {
		t.Errorf("preview head does not match start of body")
	}
	if !strings.HasSuffix(got, string(body[n-a2aLogRespTailLimit:])) {
		t.Errorf("preview tail does not match end of body")
	}
}

// deadlineWriter is a minimal gin.ResponseWriter (methods promoted from the nil
// embedded interface; none are called here) that additionally supports
// SetWriteDeadline, standing in for the underlying connection.
type deadlineWriter struct {
	gin.ResponseWriter
	deadlineSet bool
}

func (w *deadlineWriter) SetWriteDeadline(time.Time) error {
	w.deadlineSet = true
	return nil
}

// TestA2AResponseCaptureUnwrap guards the contract that http.ResponseController
// (used by clearWriteDeadline on streaming endpoints) can traverse the response
// capture wrapper to the underlying connection. Without Unwrap the controller
// stops at the wrapper, SetWriteDeadline silently no-ops, and the 40s
// WriteTimeout it must clear stays armed — cutting off long agent turns / SSE.
func TestA2AResponseCaptureUnwrap(t *testing.T) {
	under := &deadlineWriter{}
	respCap := &a2aResponseCapture{ResponseWriter: under}
	if err := http.NewResponseController(respCap).SetWriteDeadline(time.Time{}); err != nil {
		t.Fatalf("SetWriteDeadline through capture wrapper failed: %v", err)
	}
	if !under.deadlineSet {
		t.Fatal("SetWriteDeadline did not reach the underlying writer")
	}
}
