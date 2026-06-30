package router

import "testing"

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
