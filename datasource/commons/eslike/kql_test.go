package eslike

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/olivere/elastic/v7"
)

func TestCompileKQL(t *testing.T) {
	query, err := CompileKQL(`service.name: api AND log.level: "ERROR" AND http.response.status_code >= 500`, KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	boolQuery := query["bool"].(map[string]interface{})
	filters := boolQuery["filter"].([]interface{})
	if len(filters) != 3 {
		t.Fatalf("filter count = %d, want 3", len(filters))
	}

	exists, err := CompileKQL(`trace.id: *`, KQLOptions{})
	if err != nil || exists["exists"].(map[string]interface{})["field"] != "trace.id" {
		t.Fatalf("exists query = %#v, err = %v", exists, err)
	}

	wildcard, err := CompileKQL(`service: api*`, KQLOptions{CaseInsensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	prefix := wildcard["prefix"].(map[string]interface{})["service"].(map[string]interface{})
	if prefix["value"] != "api" || prefix["case_insensitive"] != true {
		t.Fatalf("prefix query = %#v", prefix)
	}
}

func TestCompileKQLTextWildcardAndDefaultField(t *testing.T) {
	text, err := CompileKQL(`message: timeout error`, KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	match := text["match"].(map[string]interface{})["message"].(map[string]interface{})
	if match["query"] != "timeout error" || match["operator"] != "and" {
		t.Fatalf("text match = %#v", match)
	}

	wildcard, err := CompileKQL(`message: *timeout*`, KQLOptions{CaseInsensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	body := wildcard["wildcard"].(map[string]interface{})["message"].(map[string]interface{})
	if body["value"] != "*timeout*" || body["case_insensitive"] != true {
		t.Fatalf("wildcard query = %#v", body)
	}

	defaultField, err := CompileKQL(`timeout`, KQLOptions{DefaultField: "message"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := defaultField["match"].(map[string]interface{})["message"]; !ok {
		t.Fatalf("default field query = %#v", defaultField)
	}
}

func TestCompileKQLQuotedWildcardIsLiteralPhrase(t *testing.T) {
	for _, value := range []string{"foo*", "*"} {
		query, err := CompileKQL(`message: "`+value+`"`, KQLOptions{})
		if err != nil {
			t.Fatal(err)
		}
		phrase, ok := query["match_phrase"].(map[string]interface{})
		if !ok || phrase["message"] != value {
			t.Fatalf("quoted wildcard %q query = %#v", value, query)
		}
	}
}

func TestCompileKQLRejectsSlashInAnyUnquotedValueWord(t *testing.T) {
	if _, err := CompileKQL(`message: timeout /foo`, KQLOptions{}); err == nil {
		t.Fatal("expected slash in a later unquoted value word to fail")
	}
	if _, err := CompileKQL(`message: "timeout /foo"`, KQLOptions{}); err != nil {
		t.Fatalf("quoted slash should be accepted: %v", err)
	}
}

func TestCompileKQLHandlesUnicodeInput(t *testing.T) {
	query, err := CompileKQL("message:\u3000服务 超时", KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	match := query["match"].(map[string]interface{})["message"].(map[string]interface{})
	if match["query"] != "服务 超时" {
		t.Fatalf("unicode match = %#v", match)
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

func TestGetFilterQueryKQLRejectsEmptyFilter(t *testing.T) {
	_, err := GetFilterQuery(&Query{FilterLanguage: "kql", Filter: "  "}, elastic.NewRangeQuery("@timestamp"))
	if err == nil {
		t.Fatal("expected empty KQL filter to fail")
	}
}

func TestCompileKQLTimeZoneOnlyForDateField(t *testing.T) {
	options := KQLOptions{TimeZone: "+08:00", dateField: "@timestamp"}
	numeric, err := CompileKQL(`bytes >= 1024`, options)
	if err != nil {
		t.Fatal(err)
	}
	numericRange := numeric["range"].(map[string]interface{})["bytes"].(map[string]interface{})
	if _, ok := numericRange["time_zone"]; ok {
		t.Fatalf("numeric range must not contain time_zone: %#v", numericRange)
	}

	date, err := CompileKQL(`@timestamp >= "2026-07-29T00:00:00"`, options)
	if err != nil {
		t.Fatal(err)
	}
	dateRange := date["range"].(map[string]interface{})["@timestamp"].(map[string]interface{})
	if dateRange["time_zone"] != "+08:00" {
		t.Fatalf("date range = %#v", dateRange)
	}
}

func TestCompileKQLRangePreservesNumericType(t *testing.T) {
	query, err := CompileKQL(`bytes >= 1024`, KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	body := query["range"].(map[string]interface{})["bytes"].(map[string]interface{})
	if value, ok := body["gte"].(int64); !ok || value != 1024 {
		t.Fatalf("integer range value = %#v (%T)", body["gte"], body["gte"])
	}

	query, err = CompileKQL(`ratio < 1.5`, KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	body = query["range"].(map[string]interface{})["ratio"].(map[string]interface{})
	if value, ok := body["lt"].(float64); !ok || value != 1.5 {
		t.Fatalf("float range value = %#v (%T)", body["lt"], body["lt"])
	}

	query, err = CompileKQL(`@timestamp >= "1700000000"`, KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	body = query["range"].(map[string]interface{})["@timestamp"].(map[string]interface{})
	if value, ok := body["gte"].(string); !ok || value != "1700000000" {
		t.Fatalf("quoted numeric range value = %#v (%T)", body["gte"], body["gte"])
	}

	query, err = CompileKQL(`@timestamp >= 1700000000`,
		KQLOptions{TimeZone: "+08:00", dateField: "@timestamp"})
	if err != nil {
		t.Fatal(err)
	}
	body = query["range"].(map[string]interface{})["@timestamp"].(map[string]interface{})
	if _, ok := body["gte"].(int64); !ok || body["time_zone"] != "+08:00" {
		t.Fatalf("numeric date range = %#v", body)
	}
}

func TestCompileKQLReportsLexerErrors(t *testing.T) {
	_, err := CompileKQL(`message: "unterminated`, KQLOptions{})
	if err == nil || !strings.Contains(err.Error(), "unterminated quoted string") {
		t.Fatalf("lexer error = %v", err)
	}
}

func TestCompileKQLRejectsExcessiveNesting(t *testing.T) {
	input := strings.Repeat("(", kqlMaxNestingDepth+1) + "a: 1" +
		strings.Repeat(")", kqlMaxNestingDepth+1)
	_, err := CompileKQL(input, KQLOptions{})
	if err == nil || !strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("nesting error = %v", err)
	}

	input = strings.Repeat("NOT ", kqlMaxNestingDepth+1) + "a: 1"
	if _, err := CompileKQL(input, KQLOptions{}); err == nil {
		t.Fatal("expected excessive unary nesting to fail")
	}
}

func TestCompileKQLBooleanComposition(t *testing.T) {
	orQuery, err := CompileKQL(`a: 1 OR b: 2 OR c: 3`, KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	orBody := orQuery["bool"].(map[string]interface{})
	if orBody["minimum_should_match"] != 1 || len(orBody["should"].([]interface{})) != 3 {
		t.Fatalf("OR query = %#v", orQuery)
	}

	if _, err := CompileKQL(`NOT NOT a: 1`, KQLOptions{}); err != nil {
		t.Fatalf("double NOT rejected: %v", err)
	}
	if _, err := CompileKQL(`(a: 1 OR (b: 2 AND c: 3))`, KQLOptions{}); err != nil {
		t.Fatalf("nested boolean expression rejected: %v", err)
	}
}

func TestCompileKQLImplicitAnd(t *testing.T) {
	query, err := CompileKQL(`status: 200 level: ERROR service: api`, KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	filters := query["bool"].(map[string]interface{})["filter"].([]interface{})
	if len(filters) != 3 {
		t.Fatalf("implicit AND filters = %#v", filters)
	}

	query, err = CompileKQL(`status: 200 NOT level: DEBUG`, KQLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	filters = query["bool"].(map[string]interface{})["filter"].([]interface{})
	if len(filters) != 2 {
		t.Fatalf("implicit AND NOT filters = %#v", filters)
	}
	if _, ok := filters[1].(map[string]interface{})["bool"]; !ok {
		t.Fatalf("implicit NOT query = %#v", filters[1])
	}
}

func TestGetFilterQueryLuceneCompatibility(t *testing.T) {
	for _, language := range []string{"", "lucene"} {
		query, err := GetFilterQuery(&Query{FilterLanguage: language, Filter: `level:ERROR`},
			elastic.NewRangeQuery("@timestamp").Gte(1000).Lt(2000))
		if err != nil {
			t.Fatal(err)
		}
		source, err := query.Source()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), "query_string") {
			t.Fatalf("language %q no longer uses Lucene query_string: %s", language, encoded)
		}
	}
}

func TestCompileKQLRejectsLuceneOnlySyntax(t *testing.T) {
	if _, err := CompileKQL(`message:/timeout.*/`, KQLOptions{}); err == nil {
		t.Fatal("expected regexp syntax to be rejected")
	}
}
