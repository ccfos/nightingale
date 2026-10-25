package eslike

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/olivere/elastic/v7"
)

type capturedSearch struct {
	indices []string
	body    struct {
		Pit *struct {
			ID        string `json:"id"`
			KeepAlive string `json:"keep_alive"`
		} `json:"pit"`
		TrackTotalHits bool                         `json:"track_total_hits"`
		Sort           []map[string]json.RawMessage `json:"sort"`
	}
}

func runQueryLog(t *testing.T, q *Query, pitID string) capturedSearch {
	t.Helper()
	var got capturedSearch
	search := func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error) {
		got.indices = indices
		raw, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &got.body); err != nil {
			t.Fatal(err)
		}
		return &elastic.SearchResult{
			PitId: pitID,
			Hits:  &elastic.SearchHits{TotalHits: &elastic.TotalHits{Value: 0, Relation: "eq"}},
		}, nil
	}
	if _, _, err := QueryLog(context.Background(), q, 10, "7.10+", 5, search); err != nil {
		t.Fatal(err)
	}
	return got
}

func sortFieldNames(sorts []map[string]json.RawMessage) []string {
	fields := make([]string, 0, len(sorts))
	for _, item := range sorts {
		for field := range item {
			fields = append(fields, field)
		}
	}
	return fields
}

func newPitQuery(searchAfter []interface{}) *Query {
	return &Query{
		Index:        "logs-*",
		DateField:    "@timestamp",
		Start:        1,
		End:          2,
		Limit:        10,
		PitID:        "pit-1",
		PitKeepAlive: "5m",
		SearchAfter: &SearchAfter{
			SortFields: []SortField{
				{Field: "@timestamp", Ascending: false},
				{Field: "_shard_doc", Ascending: true},
			},
			SearchAfter: searchAfter,
		},
	}
}

func TestQueryLogPointInTimeFirstPage(t *testing.T) {
	q := newPitQuery(nil)
	got := runQueryLog(t, q, "pit-2")

	if len(got.indices) != 0 {
		t.Fatalf("pit search must not send indices, got %#v", got.indices)
	}
	if got.body.Pit == nil || got.body.Pit.ID != "pit-1" || got.body.Pit.KeepAlive != "5m" {
		t.Fatalf("pit = %+v", got.body.Pit)
	}
	if !got.body.TrackTotalHits {
		t.Fatal("first page must track total hits")
	}
	if fields := sortFieldNames(got.body.Sort); len(fields) != 2 || fields[0] != "@timestamp" || fields[1] != "_shard_doc" {
		t.Fatalf("sort fields = %#v", fields)
	}
	if q.PitID != "pit-2" {
		t.Fatalf("pit id not refreshed: %q", q.PitID)
	}
}

func TestQueryLogPointInTimeLaterPageSkipsTotal(t *testing.T) {
	got := runQueryLog(t, newPitQuery([]interface{}{int64(100), int64(5)}), "")
	if got.body.TrackTotalHits {
		t.Fatal("later pit pages should not track total hits")
	}
}

func TestQueryLogWithoutPointInTimeKeepsIndicesAndDefaultSort(t *testing.T) {
	q := &Query{
		Index:       "logs-a,logs-b",
		DateField:   "@timestamp",
		Start:       1,
		End:         2,
		Limit:       10,
		SearchAfter: &SearchAfter{SearchAfter: []interface{}{int64(1)}},
	}
	got := runQueryLog(t, q, "")

	if len(got.indices) != 2 || got.indices[0] != "logs-a" || got.indices[1] != "logs-b" {
		t.Fatalf("indices = %#v", got.indices)
	}
	if got.body.Pit != nil {
		t.Fatalf("unexpected pit: %+v", got.body.Pit)
	}
	if !got.body.TrackTotalHits {
		t.Fatal("non-pit query must track total hits")
	}
	if fields := sortFieldNames(got.body.Sort); len(fields) != 3 || fields[1] != "_index" || fields[2] != "_id" {
		t.Fatalf("default sort fields changed: %#v", fields)
	}
}
