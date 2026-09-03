package chat

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// msgWith builds a minimal AssistantMessage for applyBackfill tests.
func msgWith(seq int64, param map[string]interface{}, resps ...models.AssistantContentType) models.AssistantMessage {
	m := models.AssistantMessage{SeqID: seq}
	m.Query.Action.Param = param
	for _, ct := range resps {
		m.Response = append(m.Response, models.AssistantMessageResponse{ContentType: ct})
	}
	return m
}

func TestApplyBackfill(t *testing.T) {
	cases := []struct {
		name     string
		missing  []string
		priorMsg []models.AssistantMessage
		curSeq   int64
		want     map[string]interface{} // expected keys+values present in dst
		absent   []string               // keys that must NOT be present in dst
	}{
		{
			name:    "bug case: inherit bg+ds from immediate prior form submit",
			missing: []string{"busi_group_id", "datasource_id"},
			priorMsg: []models.AssistantMessage{
				msgWith(1, nil),
				msgWith(2, map[string]interface{}{"busi_group_id": 1.0, "datasource_id": 5.0}),
			},
			curSeq: 3,
			want:   map[string]interface{}{"busi_group_id": 1.0, "datasource_id": 5.0},
		},
		{
			name:    "most-recent-wins",
			missing: []string{"datasource_id"},
			priorMsg: []models.AssistantMessage{
				msgWith(1, map[string]interface{}{"datasource_id": 4.0}),
				msgWith(2, map[string]interface{}{"datasource_id": 9.0}),
			},
			curSeq: 3,
			want:   map[string]interface{}{"datasource_id": 9.0},
		},
		{
			name:    "flow boundary: do NOT inherit across a completed creation",
			missing: []string{"datasource_id"},
			priorMsg: []models.AssistantMessage{
				msgWith(1, map[string]interface{}{"datasource_id": 4.0}),
				msgWith(2, nil, models.ContentTypeAlertRule), // previous creation finished here
				msgWith(3, nil),
			},
			curSeq: 4,
			absent: []string{"datasource_id"},
		},
		{
			name:    "hard boundary: do NOT inherit the boundary message's OWN param",
			missing: []string{"datasource_id"},
			priorMsg: []models.AssistantMessage{
				// one-turn creation: the form-submit param {ds} and the result
				// card land on the SAME message; a later follow-up must re-ask
				// rather than inherit the finished rule's datasource.
				msgWith(1, map[string]interface{}{"datasource_id": 4.0}, models.ContentTypeAlertRule),
			},
			curSeq: 2,
			absent: []string{"datasource_id"},
		},
		{
			name:    "inherit from within current flow (newer than the card)",
			missing: []string{"datasource_id"},
			priorMsg: []models.AssistantMessage{
				msgWith(1, map[string]interface{}{"datasource_id": 4.0}, models.ContentTypeAlertRule),
				msgWith(2, map[string]interface{}{"datasource_id": 9.0}),
			},
			curSeq: 3,
			want:   map[string]interface{}{"datasource_id": 9.0},
		},
		{
			name:    "skip in-flight / newer seq",
			missing: []string{"datasource_id"},
			priorMsg: []models.AssistantMessage{
				msgWith(1, map[string]interface{}{"datasource_id": 4.0}),
				msgWith(5, map[string]interface{}{"datasource_id": 9.0}),
			},
			curSeq: 5,
			want:   map[string]interface{}{"datasource_id": 4.0},
		},
		{
			name:    "int64 param is usable (not just float64)",
			missing: []string{"busi_group_id"},
			priorMsg: []models.AssistantMessage{
				msgWith(1, map[string]interface{}{"busi_group_id": int64(7)}),
			},
			curSeq: 2,
			want:   map[string]interface{}{"busi_group_id": int64(7)},
		},
		{
			name:    "zero/invalid value is not inherited",
			missing: []string{"datasource_id"},
			priorMsg: []models.AssistantMessage{
				msgWith(1, map[string]interface{}{"datasource_id": 0.0}),
			},
			curSeq: 2,
			absent: []string{"datasource_id"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dst := map[string]interface{}{}
			applyBackfill(dst, append([]string(nil), c.missing...), c.priorMsg, c.curSeq, "chat-test")

			for k, want := range c.want {
				got, ok := dst[k]
				if !ok {
					t.Fatalf("key %q missing from dst, want %v", k, want)
				}
				if got != want {
					t.Fatalf("dst[%q] = %v (%T), want %v (%T)", k, got, got, want, want)
				}
			}
			for _, k := range c.absent {
				if _, ok := dst[k]; ok {
					t.Fatalf("key %q should NOT be in dst, got %v", k, dst[k])
				}
			}
		})
	}
}
