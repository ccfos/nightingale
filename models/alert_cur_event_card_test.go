package models

import (
	"testing"
)

func dims(event *AlertCurEvent, rule string, t *testing.T) []CardDimension {
	t.Helper()
	d, err := event.GenCardDimensions(rule)
	if err != nil {
		t.Fatalf("GenCardDimensions(%q) err: %v", rule, err)
	}
	return d
}

func TestGenCardDimensions(t *testing.T) {
	event := &AlertCurEvent{
		GroupName: "biz",
		RuleName:  "cpu high",
		TagsJSON:  []string{"ident=host-1", "region=cn::east"},
	}

	cases := []struct {
		name      string
		rule      string
		wantDims  []CardDimension
		wantTitle string
	}{
		{
			name: "field and tagkey",
			rule: "field:group_name::tagkey:ident",
			wantDims: []CardDimension{
				{Type: "field", Field: "group_name", Value: "biz"},
				{Type: "tagkey", Field: "ident", Value: "host-1"},
			},
			wantTitle: "biz::host-1",
		},
		{
			name: "empty value falls back to Others",
			rule: "tagkey:not_exist",
			wantDims: []CardDimension{
				{Type: "tagkey", Field: "not_exist", Value: "Others"},
			},
			wantTitle: "Others",
		},
		{
			name: "tag value contains the :: separator",
			rule: "field:group_name::tagkey:region",
			wantDims: []CardDimension{
				{Type: "field", Field: "group_name", Value: "biz"},
				{Type: "tagkey", Field: "region", Value: "cn::east"},
			},
			wantTitle: "biz::cn::east",
		},
		{
			name: "go template rule renders one dimension",
			rule: "{{.RuleName}}",
			wantDims: []CardDimension{
				{Type: "template", Field: "", Value: "cpu high"},
			},
			wantTitle: "cpu high",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dims(event, c.rule, t)
			if !cardDimensionsEqual(got, c.wantDims) {
				t.Fatalf("dims = %+v, want %+v", got, c.wantDims)
			}

			title, err := event.GenCardTitle(c.rule)
			if err != nil {
				t.Fatalf("GenCardTitle err: %v", err)
			}
			if title != c.wantTitle {
				t.Fatalf("title = %q, want %q", title, c.wantTitle)
			}
		})
	}
}

// 两个事件的卡片标题可能因为维度值含 "::" 而拼接出相同字符串，但其结构化维度不同，
// 不能被当作同一张卡片——这是采用结构化 dimensions 而非拼接标题的根本原因。
func TestCardDimensionsDistinguishAmbiguousTitle(t *testing.T) {
	rule := "field:group_name::tagkey:ident"

	a := &AlertCurEvent{GroupName: "a::b", TagsJSON: []string{"ident=c"}}
	b := &AlertCurEvent{GroupName: "a", TagsJSON: []string{"ident=b::c"}}

	titleA, _ := a.GenCardTitle(rule)
	titleB, _ := b.GenCardTitle(rule)
	if titleA != titleB {
		t.Fatalf("expected colliding titles, got %q vs %q", titleA, titleB)
	}

	if cardDimensionsEqual(dims(a, rule, t), dims(b, rule, t)) {
		t.Fatal("events with different dimensions must not be treated as the same card")
	}
}

func TestCardDimensionsEqual(t *testing.T) {
	base := []CardDimension{
		{Type: "field", Field: "group_name", Value: "biz"},
		{Type: "tagkey", Field: "ident", Value: "host-1"},
	}

	cases := []struct {
		name string
		b    []CardDimension
		want bool
	}{
		{"identical", []CardDimension{{Type: "field", Field: "group_name", Value: "biz"}, {Type: "tagkey", Field: "ident", Value: "host-1"}}, true},
		{"different length", base[:1], false},
		{"different value", []CardDimension{{Type: "field", Field: "group_name", Value: "biz"}, {Type: "tagkey", Field: "ident", Value: "host-2"}}, false},
		{"different field", []CardDimension{{Type: "field", Field: "group_name", Value: "biz"}, {Type: "tagkey", Field: "app", Value: "host-1"}}, false},
		{"different type", []CardDimension{{Type: "tagkey", Field: "group_name", Value: "biz"}, {Type: "tagkey", Field: "ident", Value: "host-1"}}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cardDimensionsEqual(base, c.b); got != c.want {
				t.Fatalf("cardDimensionsEqual = %v, want %v", got, c.want)
			}
		})
	}
}
