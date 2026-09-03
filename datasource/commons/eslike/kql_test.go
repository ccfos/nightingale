package eslike

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/olivere/elastic/v7"
)

func TestCompileKQLFrontendCompatibility(t *testing.T) {
	cases := []struct {
		filter string
		want   map[string]interface{}
	}{
		{`message: (timeout error)`, should(map[string]interface{}{"match": map[string]interface{}{"message": "timeout error"}})},
		{`message: "timeout error"`, should(map[string]interface{}{"match_phrase": map[string]interface{}{"message": "timeout error"}})},
		{`message: foo~2`, should(map[string]interface{}{"match": map[string]interface{}{"message": "foo~2"}})},
		{`message: /timeout.*/`, should(map[string]interface{}{"query_string": map[string]interface{}{"fields": []string{"message"}, "query": `\/timeout.*\/`}})},
		{`field.*: logs`, should(map[string]interface{}{"match": map[string]interface{}{"field.*": "logs"}})},
		{`bytes.* >= 1024`, should(map[string]interface{}{"range": map[string]interface{}{"bytes.*": map[string]interface{}{"gte": "1024"}}})},
		{`bytes >= *`, should(map[string]interface{}{"range": map[string]interface{}{"bytes": map[string]interface{}{"gte": "*"}}})},
		{`bytes >= foo*`, should(map[string]interface{}{"range": map[string]interface{}{"bytes": map[string]interface{}{"gte": "foo*"}}})},
		{`bytes >= *foo`, should(map[string]interface{}{"range": map[string]interface{}{"bytes": map[string]interface{}{"gte": "*foo"}}})},
		{`trace.id: *`, should(map[string]interface{}{"exists": map[string]interface{}{"field": "trace.id"}})},
		{`*: (*)`, map[string]interface{}{"match_all": map[string]interface{}{}}},
		{`field: true`, should(map[string]interface{}{"match": map[string]interface{}{"field": true}})},
		{`field: null`, should(map[string]interface{}{"match": map[string]interface{}{"field": nil}})},
		{`field: \u4e2d`, should(map[string]interface{}{"match": map[string]interface{}{"field": "中"}})},
		{`foo bar`, map[string]interface{}{"multi_match": map[string]interface{}{"type": "best_fields", "query": "foo bar", "lenient": true}}},
		{`*: *`, map[string]interface{}{"match_all": map[string]interface{}{}}},
		{`user:{ first: "Alice" AND last: "White" }`, map[string]interface{}{"nested": map[string]interface{}{
			"path": "user", "score_mode": "none", "query": map[string]interface{}{"bool": map[string]interface{}{"filter": []interface{}{
				should(map[string]interface{}{"match_phrase": map[string]interface{}{"user.first": "Alice"}}),
				should(map[string]interface{}{"match_phrase": map[string]interface{}{"user.last": "White"}}),
			}}},
		}}},
		{`a: 1 OR b: 2`, map[string]interface{}{"bool": map[string]interface{}{"should": []interface{}{
			should(map[string]interface{}{"match": map[string]interface{}{"a": "1"}}),
			should(map[string]interface{}{"match": map[string]interface{}{"b": "2"}}),
		}, "minimum_should_match": 1}}},
	}
	for _, tc := range cases {
		got, err := CompileKQL(tc.filter, KQLOptions{})
		if err != nil {
			t.Fatalf("%q: %v", tc.filter, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q:\n got: %#v\nwant: %#v", tc.filter, got, tc.want)
		}
	}
}

func TestCompileKQLLeadingWildcardsAndSyntax(t *testing.T) {
	for _, filter := range []string{`message: *timeout`, `*timeout`, `message: **`} {
		if _, err := CompileKQL(filter, KQLOptions{}); err == nil || !strings.Contains(err.Error(), "Leading wildcards") {
			t.Errorf("%q error = %v, want leading wildcard error", filter, err)
		}
	}
	for _, filter := range []string{`message: *`, `*: *`, `field.*: logs`, `message: foo^2`, `message: [one TO two]`} {
		if _, err := CompileKQL(filter, KQLOptions{}); err != nil {
			t.Errorf("%q rejected: %v", filter, err)
		}
	}
	for _, filter := range []string{`status: 200 level: ERROR`, `message: foo NOT bar`, `message: "unterminated`, `a: (b: 1)`} {
		if _, err := CompileKQL(filter, KQLOptions{}); err == nil {
			t.Errorf("%q accepted", filter)
		}
	}
}

func TestCompileKQLFieldLeadingWildcardMatchesFrontendDefault(t *testing.T) {
	for _, filter := range []string{`*: foo`, `*timeout: foo`} {
		if _, err := CompileKQL(filter, KQLOptions{}); err != nil {
			t.Errorf("%q rejected: %v", filter, err)
		}
	}
}

func TestCompileKQLWildcardMarkerLiteralFieldDoesNotBecomeWildcard(t *testing.T) {
	query, err := CompileKQL(`@kuery-wildcard@: foo`, KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	match := query["bool"].(map[string]interface{})["should"].([]interface{})[0].(map[string]interface{})["match"].(map[string]interface{})
	if match[kqlWildcardMarker] != "foo" {
		t.Fatalf("marker field was changed to a wildcard: %#v", match)
	}
}

// This matrix follows Elastic's documented KQL forms and the frontend
// buildESQueryFromKuery grammar used as this compiler's compatibility target.
// The cases are kept grouped by grammar production so adding one feature does
// not silently narrow another production's accepted syntax.
func TestCompileKQLSyntaxCoverage(t *testing.T) {
	accepted := map[string][]string{
		"exists and match": {
			`http.request.method: *`, `http.request.method: GET`, `null pointer`,
			`message: null pointer`, `message: "null pointer"`, `enabled: true`,
			`enabled: false`, `field: null`,
		},
		"escaping and literals": {
			`http.request.referrer: https\://example.com`, `message: foo\*`,
			`message: "a (quoted): value*"`, `message: \u4e2d`,
			`message: \t`, `message: \r`, `message: \n`,
		},
		"range": {
			`bytes > 10000`, `bytes >= 10000`, `bytes < 20000`, `bytes <= 20000`,
			`@timestamp < now-2w`, `ip >= 10.0.0.1`, `bytes >= foo*`,
			`datastream.* >= logs`,
		},
		"wildcards": {
			`http.response.status_code: 4*`, `message: foo*bar`, `field: *`,
			`datastream.*: logs`, `*: foo`, `*timeout: foo`, `*:*`,
		},
		"boolean and grouping": {
			`NOT http.request.method: GET`, `a: 1 AND b: 2`, `a: 1 or b: 2`,
			`(a: 1 AND b: 2) OR (a: 3 AND b: 4)`,
			`http.request.method: (GET OR POST OR DELETE)`,
			`message: (timeout error)`, `message: (timeout AND error)`,
			`message: (NOT timeout)`,
		},
		"nested": {
			`user:{ first: "Alice" AND last: "White" }`,
			`user.names:{ first: "Alice" AND last: "White" }`,
			`user:{ names:{ first: Alice AND last: White } }`,
		},
	}
	for group, filters := range accepted {
		for _, filter := range filters {
			if _, err := CompileKQL(filter, KQLOptions{}); err != nil {
				t.Errorf("%s: %q rejected: %v", group, filter, err)
			}
		}
	}

	rejected := []string{
		`message: *timeout`, `*timeout`, `message: **`, // leading value wildcards
		`status: 200 level: ERROR`, // no implicit AND in frontend grammar
		`message: foo NOT bar`,     // NOT is an operator, not a value word
		`message: "unterminated`, `a: (b: 1)`, `a: 1 AND`,
		`user:{ first: Alice`, `a: (b OR c`,
	}
	for _, filter := range rejected {
		if _, err := CompileKQL(filter, KQLOptions{}); err == nil {
			t.Errorf("%q accepted", filter)
		}
	}
}

func TestCompileKQLRangeOperatorsAndEscapesDSL(t *testing.T) {
	for _, tc := range []struct {
		filter, operator, value string
	}{
		{`bytes > 1`, "gt", "1"}, {`bytes >= 1`, "gte", "1"},
		{`bytes < 1`, "lt", "1"}, {`bytes <= 1`, "lte", "1"},
	} {
		query, err := CompileKQL(tc.filter, KQLOptions{})
		if err != nil {
			t.Fatal(err)
		}
		body := query["bool"].(map[string]interface{})["should"].([]interface{})[0].(map[string]interface{})["range"].(map[string]interface{})["bytes"].(map[string]interface{})
		if len(body) != 1 || body[tc.operator] != tc.value {
			t.Errorf("%q range = %#v", tc.filter, body)
		}
	}

	query, err := CompileKQL(`http.request.referrer: https\://example.com`, KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	match := query["bool"].(map[string]interface{})["should"].([]interface{})[0].(map[string]interface{})["match"].(map[string]interface{})
	if match["http.request.referrer"] != "https://example.com" {
		t.Fatalf("escaped URL match = %#v", match)
	}
}

func TestGetFilterQueryKQLIncludesTimeRange(t *testing.T) {
	q := elastic.NewRangeQuery("@timestamp").Gte(1000).Lt(2000).Format("epoch_millis")
	query, err := GetFilterQuery(&Query{FilterLanguage: "kql", Filter: `level: ERROR`}, q)
	if err != nil {
		t.Fatal(err)
	}
	source, err := query.Source()
	if err != nil {
		t.Fatal(err)
	}
	filters := source.(map[string]interface{})["bool"].(map[string]interface{})["filter"].([]interface{})
	if len(filters) != 2 {
		t.Fatalf("filter count = %d, want 2", len(filters))
	}
}

func TestGetFilterQueryLuceneCompatibility(t *testing.T) {
	for _, language := range []string{"", "lucene", "LUCENE"} {
		query, err := GetFilterQuery(&Query{FilterLanguage: language, Filter: `level:ERROR`},
			elastic.NewRangeQuery("@timestamp").Gte(1000).Lt(2000))
		if err != nil {
			t.Fatal(err)
		}
		source, err := query.Source()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := source.(map[string]interface{})["bool"].(map[string]interface{})["filter"]; !ok {
			t.Fatalf("language %q legacy query changed: %#v", language, source)
		}
		encoded, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), "query_string") {
			t.Fatalf("language %q no longer uses Lucene query_string: %s", language, encoded)
		}
	}

	if _, err := GetFilterQuery(&Query{FilterLanguage: "sql", Filter: `level:ERROR`},
		elastic.NewRangeQuery("@timestamp")); err == nil {
		t.Fatal("expected an unsupported filter_language to fail")
	}
}

// 空过滤条件在 KQL 下与 Lucene 同义：只按时间范围查全部，而不是报错。
func TestGetFilterQueryKQLEmptyFilterFallsBackToTimeRangeOnly(t *testing.T) {
	want, err := GetQueryString("", elastic.NewRangeQuery("@timestamp")).Source()
	if err != nil {
		t.Fatal(err)
	}
	encodedWant, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	for _, filter := range []string{"", "  "} {
		query, err := GetFilterQuery(&Query{FilterLanguage: "kql", Filter: filter},
			elastic.NewRangeQuery("@timestamp"))
		if err != nil {
			t.Fatalf("empty KQL filter %q must not fail: %v", filter, err)
		}
		source, err := query.Source()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != string(encodedWant) {
			t.Fatalf("empty KQL filter %q: got %s, want %s", filter, encoded, encodedWant)
		}
	}
}

func TestCompileKQLTimeZoneOnlyForDateField(t *testing.T) {
	options := KQLOptions{TimeZone: "+08:00", dateField: "@timestamp"}
	numeric, err := CompileKQL(`bytes >= 1024`, options)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := kqlRangeBody(t, numeric, "bytes")["time_zone"]; ok {
		t.Fatalf("numeric range must not carry time_zone: %#v", numeric)
	}

	date, err := CompileKQL(`@timestamp >= "2026-07-29T00:00:00"`, options)
	if err != nil {
		t.Fatal(err)
	}
	if body := kqlRangeBody(t, date, "@timestamp"); body["time_zone"] != "+08:00" {
		t.Fatalf("date range = %#v", body)
	}

	withoutZone, err := CompileKQL(`@timestamp >= "2026-07-29T00:00:00"`, KQLOptions{dateField: "@timestamp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := kqlRangeBody(t, withoutZone, "@timestamp")["time_zone"]; ok {
		t.Fatalf("range must not carry time_zone when none was requested: %#v", withoutZone)
	}
}

func TestCompileKQLNestingDepthLimit(t *testing.T) {
	deep := map[string]string{
		"parentheses": strings.Repeat("(", kqlMaxNestingDepth+1) + "a: 1" + strings.Repeat(")", kqlMaxNestingDepth+1),
		"not":         strings.Repeat("NOT ", kqlMaxNestingDepth+1) + "a: 1",
		"nested":      strings.Repeat("a:{ ", kqlMaxNestingDepth+1) + "b: 1" + strings.Repeat(" }", kqlMaxNestingDepth+1),
	}
	for name, filter := range deep {
		if _, err := CompileKQL(filter, KQLOptions{}); err == nil || !strings.Contains(err.Error(), "maximum depth") {
			t.Errorf("%s nesting error = %v, want maximum depth error", name, err)
		}
	}

	shallow := strings.Repeat("(", kqlMaxNestingDepth-1) + "a: 1" + strings.Repeat(")", kqlMaxNestingDepth-1)
	if _, err := CompileKQL(shallow, KQLOptions{}); err != nil {
		t.Fatalf("nesting below the limit rejected: %v", err)
	}
}

func TestCompileKQLMultiWordWildcardValue(t *testing.T) {
	for _, tc := range []struct{ filter, query string }{
		{`message: foo bar*`, "foo bar*"},
		{`message: foo* bar`, "foo* bar"},
	} {
		query, err := CompileKQL(tc.filter, KQLOptions{})
		if err != nil {
			t.Fatalf("%q: %v", tc.filter, err)
		}
		want := should(map[string]interface{}{"query_string": map[string]interface{}{
			"fields": []string{"message"}, "query": tc.query}})
		if !reflect.DeepEqual(query, want) {
			t.Errorf("%q:\n got: %#v\nwant: %#v", tc.filter, query, want)
		}
	}

	if _, err := CompileKQL(`message: * foo`, KQLOptions{}); err == nil ||
		!strings.Contains(err.Error(), "Leading wildcards") {
		t.Errorf("leading wildcard in a multi-word value = %v", err)
	}
}

func TestCompileKQLErrorsQuoteTheOriginalText(t *testing.T) {
	_, err := CompileKQL(`message: "foo" bar*`, KQLOptions{})
	if err == nil {
		t.Fatal("expected a value after a quoted string to fail")
	}
	if strings.Contains(err.Error(), kqlWildcardMarker) {
		t.Fatalf("error leaks the internal wildcard marker: %v", err)
	}
	if !strings.Contains(err.Error(), `"bar*"`) {
		t.Fatalf("error does not quote the original text: %v", err)
	}
}

func kqlRangeBody(t *testing.T, query map[string]interface{}, field string) map[string]interface{} {
	t.Helper()
	should := query["bool"].(map[string]interface{})["should"].([]interface{})
	return should[0].(map[string]interface{})["range"].(map[string]interface{})[field].(map[string]interface{})
}

func should(query map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"bool": map[string]interface{}{"should": []interface{}{query}, "minimum_should_match": 1}}
}
