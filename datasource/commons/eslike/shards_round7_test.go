package eslike

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/require"
)

func failedShardResult(t *testing.T, hits string) *elastic.SearchResult {
	t.Helper()
	raw := `{
  "_shards":{"total":3,"failed":2,"failures":[
   {"index":"logs-a","shard":0,"reason":{"type":"query_error","reason":"bad field"}},
   {"index":"logs-b","shard":1,"reason":{"reason":"missing mapping"}}
  ]},
  "hits":{"total":{"value":1,"relation":"eq"},"hits":` + hits + `}
 }`
	var result elastic.SearchResult
	require.NoError(t, json.Unmarshal([]byte(raw), &result))
	return &result
}

func TestQueryLog_ReturnsShardErrorsWhenNoHitsAreAvailable(t *testing.T) {
	result := failedShardResult(t, `[]`)
	search := func(context.Context, []string, interface{}, int, int) (*elastic.SearchResult, error) {
		return result, nil
	}

	logs, total, err := QueryLog(context.Background(), round2Query(), 1000, "7.17.0", 3, search)

	require.ErrorContains(t, err, "2/3 failed")
	require.ErrorContains(t, err, "index=logs-a shard=0: query_error: bad field")
	require.ErrorContains(t, err, "index=logs-b shard=1: missing mapping")
	require.Nil(t, logs)
	require.Zero(t, total)
}

func TestQueryLog_ReturnsPartialHitsDespiteShardFailure(t *testing.T) {
	result := failedShardResult(t, `[{"_id":"1","_source":{"message":"available"}}]`)
	search := func(context.Context, []string, interface{}, int, int) (*elastic.SearchResult, error) {
		return result, nil
	}

	logs, total, err := QueryLog(context.Background(), round2Query(), 1000, "7.17.0", 3, search)

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	hit := logs[0].(*elastic.SearchHit)
	require.Equal(t, "1", hit.Id)
	require.JSONEq(t, `{"message":"available"}`, string(hit.Source))
}

func TestQueryData_RejectsShardFailuresBeforeParsingAggregations(t *testing.T) {
	result := failedShardResult(t, `[]`)
	search := func(context.Context, []string, interface{}, int, int) (*elastic.SearchResult, error) {
		return result, nil
	}

	data, err := QueryData(context.Background(), round2Query(), 1000, "7.17.0", search)

	require.ErrorContains(t, err, "query_error: bad field")
	require.Nil(t, data)
}

func TestIterGetMap_FlattensNestedFieldsUnderPrefix(t *testing.T) {
	result := map[string]interface{}{"existing": "retained"}
	input := map[string]interface{}{
		"host":  map[string]interface{}{"name": "api", "tags": []interface{}{"prod", "eu"}},
		"count": json.Number("3"),
	}

	IterGetMap(input, result, "event")

	require.Equal(t, map[string]interface{}{
		"existing": "retained", "event.host.name": []interface{}{"api"},
		"event.host.tags": []interface{}{[]interface{}{"prod", "eu"}}, "event.count": []interface{}{json.Number("3")},
	}, result)
}

func TestHitFilter_OnlyAllowsAggregatableScalarTypes(t *testing.T) {
	for _, typ := range []string{
		"keyword", "date", "long", "integer", "short", "byte", "double", "float",
		"half_float", "scaled_float", "unsigned_long",
	} {
		t.Run(typ, func(t *testing.T) { require.False(t, HitFilter(typ)) })
	}
	for _, typ := range []string{"text", "object", "nested", "unknown", ""} {
		t.Run(typ, func(t *testing.T) { require.True(t, HitFilter(typ)) })
	}
}
