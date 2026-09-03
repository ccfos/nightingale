package tools

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func TestConfigPatch(t *testing.T) {
	patch, keys, err := configPatch(`{"disabled":1,"cause":"维护窗口","tags":[]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"cause", "disabled", "tags"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	if _, ok := patch["disabled"]; !ok {
		t.Fatalf("patch missing disabled key")
	}

	// id/group_id 不是可应用的改动：不进 keys（确认文案/updated），但留在 patch 里给 guard 用。
	patch, keys, err = configPatch(`{"id":7,"group_id":2,"disabled":1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"disabled"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	if _, ok := patch["group_id"]; !ok {
		t.Fatalf("patch should keep group_id for patchGroupIDGuard")
	}

	if _, _, err := configPatch(`[{"disabled":1}]`); err == nil {
		t.Fatalf("expected error for non-object config (array)")
	}
	if _, _, err := configPatch(`not json`); err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestRejectEmptyArrayPatch(t *testing.T) {
	cases := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{"field absent", `{"disabled":1}`, false},
		{"non-empty array", `{"severities":[1,2]}`, false},
		{"empty array rejected", `{"severities":[]}`, true},
		// null 同样把 merge 后的切片置空，落库被 FE2DB 静默丢弃，一并拒绝。
		{"null rejected", `{"severities":null}`, true},
		{"non-array value ignored", `{"severities":5}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			patch, _, err := configPatch(c.config)
			if err != nil {
				t.Fatalf("configPatch: %v", err)
			}
			err = rejectEmptyArrayPatch(patch, "severities")
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, c.wantErr)
			}
		})
	}
}

func TestPatchGroupIDGuard(t *testing.T) {
	cases := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{"no group_id", `{"disabled":1}`, false},
		{"same group_id", `{"group_id":2}`, false},
		{"zero group_id treated as unset", `{"group_id":0}`, false},
		{"different group_id rejected", `{"group_id":3}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			patch, _, err := configPatch(c.config)
			if err != nil {
				t.Fatalf("configPatch: %v", err)
			}
			err = patchGroupIDGuard(patch, 2)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, c.wantErr)
			}
		})
	}
}

// TestMutePatchMergeSemantics pins the merge contract update_alert_mute relies
// on: a second Unmarshal onto the existing FE-shape struct only overwrites
// fields present in the patch, and arrays are replaced wholesale.
func TestMutePatchMergeSemantics(t *testing.T) {
	existing := models.AlertMute{
		Id:                7,
		GroupId:           2,
		Note:              "old note",
		Cause:             "old cause",
		Btime:             1000,
		Etime:             2000,
		Disabled:          0,
		SeveritiesJson:    []int{1, 2, 3},
		DatasourceIdsJson: []int64{0},
	}

	merged := existing
	if err := json.Unmarshal([]byte(`{"cause":"new cause","disabled":1,"severities":[1]}`), &merged); err != nil {
		t.Fatalf("merge unmarshal: %v", err)
	}

	if merged.Cause != "new cause" || merged.Disabled != 1 {
		t.Fatalf("patched fields not applied: %+v", merged)
	}
	if !reflect.DeepEqual(merged.SeveritiesJson, []int{1}) {
		t.Fatalf("severities should be replaced wholesale, got %v", merged.SeveritiesJson)
	}
	// Fields absent from the patch keep their values.
	if merged.Note != "old note" || merged.Btime != 1000 || merged.Etime != 2000 {
		t.Fatalf("unpatched fields were clobbered: %+v", merged)
	}
	if !reflect.DeepEqual(merged.DatasourceIdsJson, []int64{0}) {
		t.Fatalf("datasource_ids should be untouched, got %v", merged.DatasourceIdsJson)
	}
}
