package router

import "strings"

// reactStreamDemuxer splits a streamed ReAct raw LLM output into two channels
// at the final-answer boundary: everything before goes to the reason channel
// (thoughts), everything after goes to the content channel (the user-facing
// body).
//
// Without this split, ReAct's content would only reach the wire as one big
// chunk at StreamTypeDone time — defeating streaming for the very portion
// users wait to see. Direct mode already streams its tokens straight to
// content; this brings ReAct in line.
//
// Two ReAct forms are recognised (mirroring aiagent.parseReActResponse):
//
//   - shorthand: "Final Answer: <body>"
//   - canonical: "Action: Final Answer\nAction Input: <body>"
//
// Two correctness requirements drove the design:
//
//  1. A marker must be detected even when the LLM streams it byte by byte.
//     We retain the longest suffix of already-seen bytes that is a prefix of
//     ANY marker and recheck (tail + new delta) on every Feed call.
//
//  2. Marker text must never leak into the reason channel — not even the
//     first few characters. Otherwise users would see stray "Final" /
//     "Action: Fin" tokens in the thinking panel. We emit only bytes we KNOW
//     can't be part of any marker; ambiguous suffix bytes are buffered in
//     `tail` until a subsequent delta resolves them. Latency added to the
//     reason channel is at most len(longestMarker)-1 bytes — negligible
//     against LLM token rate.
type reactStreamDemuxer struct {
	finalSeen bool
	tail      strings.Builder
	// skipNextSpace is set when a marker is detected at the very end of a
	// delta (no post-marker text yet). The first byte of the NEXT delta is
	// the body's leading space, which should be stripped to match how
	// aiagent.parseReActResponse normalises the body.
	skipNextSpace bool
}

// reactFinalAnswerMarkers lists the boundary strings whose appearance flips
// the demuxer from reason mode into content mode. Order doesn't matter for
// correctness — findFirstMarker picks the earliest hit — but for tail
// retention longer markers extend the worst-case prefix we must remember.
var reactFinalAnswerMarkers = []string{
	"Action: Final Answer\nAction Input:",
	"Final Answer:",
}

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

	if idx, markerLen := findFirstMarker(combined); idx >= 0 {
		d.finalSeen = true
		reason = combined[:idx]
		post := combined[idx+markerLen:]
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
	// become the start of any marker; emit everything before it as reason.
	// This prevents partial-marker bytes ("Final Answ...", "Action: Fin...")
	// from ever appearing in the reason channel.
	k := longestMarkerPrefixSuffix(combined)
	if k == 0 {
		return combined, ""
	}
	d.tail.WriteString(combined[len(combined)-k:])
	return combined[:len(combined)-k], ""
}

// findFirstMarker returns the earliest marker hit in s along with the length
// of the matched marker, or (-1, 0) when none is present. Earliest-wins is
// the right policy because once a marker is crossed, everything after is
// content — we never want to skip past an earlier boundary to honor a later
// one.
func findFirstMarker(s string) (idx, markerLen int) {
	bestIdx := -1
	bestLen := 0
	for _, m := range reactFinalAnswerMarkers {
		i := strings.Index(s, m)
		if i < 0 {
			continue
		}
		if bestIdx == -1 || i < bestIdx {
			bestIdx = i
			bestLen = len(m)
		}
	}
	return bestIdx, bestLen
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
// of s form a prefix of ANY marker in reactFinalAnswerMarkers. Markers are
// ASCII, so this is a byte-level check — multi-byte UTF-8 in the LLM output
// can't accidentally match (UTF-8 continuation bytes have the high bit set;
// markers only contain ASCII printable chars plus '\n').
func longestMarkerPrefixSuffix(s string) int {
	best := 0
	for _, m := range reactFinalAnswerMarkers {
		maxK := len(m) - 1
		if maxK > len(s) {
			maxK = len(s)
		}
		// Walk k downward but stop as soon as we'd no longer beat `best` —
		// no point checking shorter suffixes when a longer match for another
		// marker has already won.
		for k := maxK; k > best; k-- {
			if strings.HasPrefix(m, s[len(s)-k:]) {
				best = k
				break
			}
		}
	}
	return best
}
