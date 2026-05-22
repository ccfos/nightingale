package router

import "strings"

// reactStreamDemuxer splits a streamed ReAct raw LLM output into two channels
// at the "Final Answer:" boundary: everything before goes to the reason
// channel (thoughts), everything after goes to the content channel (the
// user-facing body).
//
// Without this split, ReAct's content would only reach the wire as one big
// chunk at StreamTypeDone time — defeating streaming for the very portion
// users wait to see. Direct mode already streams its tokens straight to
// content; this brings ReAct in line.
//
// Two correctness requirements drove the design:
//
//  1. The marker must be detected even when the LLM streams it byte by byte.
//     We achieve this by retaining the longest suffix of already-seen bytes
//     that is a prefix of the marker, and checking (tail + new delta) on
//     every Feed call.
//
//  2. The marker text itself must never leak into the reason channel — even
//     the first few characters. Otherwise users would see stray "Final" or
//     "Final Answ" tokens in the thinking panel. We achieve this by emitting
//     only the bytes we KNOW can't be part of a marker; ambiguous suffix
//     bytes are buffered in `tail` until a subsequent delta resolves them.
//     This adds at most len(marker)-1 = 12 bytes of latency to the reason
//     channel — negligible against LLM token rate.
type reactStreamDemuxer struct {
	finalSeen bool
	tail      strings.Builder
	// skipNextSpace is set when the marker is detected at the very end of a
	// delta (no post-marker text yet). The first byte of the NEXT delta is
	// the body's leading space, which should be stripped to match how
	// aiagent.parseReActResponse normalises "Final Answer: <body>" to "<body>".
	skipNextSpace bool
}

const reactFinalAnswerMarker = "Final Answer:"

// Feed consumes one stream delta and returns the portion to push as reason
// and the portion to push as content. Either or both may be empty. Once the
// marker has been seen, every subsequent byte flows to content.
func (d *reactStreamDemuxer) Feed(delta string) (reason, content string) {
	if d.finalSeen {
		if d.skipNextSpace {
			d.skipNextSpace = false
			delta = strings.TrimPrefix(delta, " ")
		}
		return "", delta
	}

	combined := d.tail.String() + delta
	d.tail.Reset()

	if idx := strings.Index(combined, reactFinalAnswerMarker); idx >= 0 {
		d.finalSeen = true
		reason = combined[:idx]
		post := combined[idx+len(reactFinalAnswerMarker):]
		if post == "" {
			// Body hasn't started yet — defer the leading-space strip to the
			// next delta. Without this, content would start with a leading
			// blank when the marker arrives at the tail of a chunk.
			d.skipNextSpace = true
			return reason, ""
		}
		return reason, strings.TrimPrefix(post, " ")
	}

	// No marker yet. Retain the longest suffix of combined that could still
	// become the start of a marker; emit everything before it as reason.
	// This prevents partial-marker bytes ("Final Answ...") from ever
	// appearing in the reason channel.
	k := longestMarkerPrefixSuffix(combined)
	if k == 0 {
		return combined, ""
	}
	d.tail.WriteString(combined[len(combined)-k:])
	return combined[:len(combined)-k], ""
}

// Reset clears state for the next ReAct iteration. The router calls this on
// StreamTypeToolResult — each iteration is a fresh LLM call whose output
// starts with a new "Thought:" and may eventually emit its own marker.
func (d *reactStreamDemuxer) Reset() {
	d.finalSeen = false
	d.skipNextSpace = false
	d.tail.Reset()
}

// FinalSeen reports whether the current iteration has crossed the marker.
// The router checks this on StreamTypeDone: a true value means content was
// already streamed out of the raw stream and Done.Content would just
// duplicate it; false means the LLM never emitted a marker (the unstructured
// fallback in react.go's parser) and Done.Content is the only signal of
// what the answer is.
func (d *reactStreamDemuxer) FinalSeen() bool {
	return d.finalSeen
}

// longestMarkerPrefixSuffix returns the largest k such that the last k bytes
// of s form a prefix of reactFinalAnswerMarker. The marker is ASCII, so this
// is a byte-level check — multi-byte UTF-8 in the LLM output can't
// accidentally match (UTF-8 continuation bytes have the high bit set; the
// marker only contains ASCII printable chars).
func longestMarkerPrefixSuffix(s string) int {
	maxK := len(reactFinalAnswerMarker) - 1
	if maxK > len(s) {
		maxK = len(s)
	}
	for k := maxK; k > 0; k-- {
		if strings.HasPrefix(reactFinalAnswerMarker, s[len(s)-k:]) {
			return k
		}
	}
	return 0
}
