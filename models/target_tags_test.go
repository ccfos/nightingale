package models

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTargetFillTagsMapCachesSourcesSeparately(t *testing.T) {
	target := &Target{
		Tags:     "custom=custom-value duplicate=custom-value ",
		HostTags: []string{"reported=reported-value", "duplicate=reported-value"},
	}

	target.FillTagsMap()

	if want := map[string]string{
		"custom":    "custom-value",
		"duplicate": "custom-value",
	}; !reflect.DeepEqual(target.UserTagsMap, want) {
		t.Fatalf("unexpected user tags: got %v, want %v", target.UserTagsMap, want)
	}
	if want := map[string]string{
		"reported":  "reported-value",
		"duplicate": "reported-value",
	}; !reflect.DeepEqual(target.HostTagsMap, want) {
		t.Fatalf("unexpected host tags: got %v, want %v", target.HostTagsMap, want)
	}
	if want := map[string]string{
		"custom":    "custom-value",
		"reported":  "reported-value",
		"duplicate": "reported-value",
	}; !reflect.DeepEqual(target.TagsMap, want) {
		t.Fatalf("unexpected merged tags: got %v, want %v", target.TagsMap, want)
	}
}

func TestTargetFillTagsMapsFromAPIFields(t *testing.T) {
	target := &Target{
		TagsJSON: []string{"custom=custom-value", "duplicate=custom-value"},
		HostTags: []string{"reported=reported-value", "duplicate=reported-value"},
	}

	target.fillTagsMaps()

	if target.UserTagsMap["custom"] != "custom-value" {
		t.Fatalf("user tags were not rebuilt from TagsJSON: %v", target.UserTagsMap)
	}
	if target.HostTagsMap["reported"] != "reported-value" {
		t.Fatalf("host tags were not rebuilt from HostTags: %v", target.HostTagsMap)
	}
	if target.TagsMap["duplicate"] != "reported-value" {
		t.Fatalf("host tag precedence was not preserved: %v", target.TagsMap)
	}
}

func TestTargetInternalTagMapsAreNotSerialized(t *testing.T) {
	target := &Target{
		Tags:     "custom=custom-value ",
		HostTags: []string{"reported=reported-value"},
	}
	target.FillTagsMap()

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("marshal target: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal target JSON: %v", err)
	}

	for _, key := range []string{"UserTagsMap", "HostTagsMap", "user_tags_map", "host_tags_map"} {
		if _, exists := got[key]; exists {
			t.Fatalf("internal tag cache %q leaked into target JSON: %s", key, data)
		}
	}
	if _, exists := got["tags_maps"]; !exists {
		t.Fatalf("existing tags_maps field is missing from target JSON: %s", data)
	}
	if _, exists := got["host_tags"]; !exists {
		t.Fatalf("existing host_tags field is missing from target JSON: %s", data)
	}
}
