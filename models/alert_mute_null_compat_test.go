package models

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/stretchr/testify/require"
)

func TestAlertMuteNullTagsWrite(t *testing.T) {
	const configuredTags = `[{"key":"ident","func":"==","value":"web01"}]`
	tests := []struct {
		name string
		tags ormx.JSONArr
		want string
	}{
		{name: "omitted", want: "[]"},
		{name: "empty", tags: ormx.JSONArr{}, want: "[]"},
		{name: "json_null", tags: ormx.JSONArr("null"), want: "[]"},
		{name: "json_null_with_whitespace", tags: ormx.JSONArr(" \nnull\t"), want: "[]"},
		{name: "empty_array", tags: ormx.JSONArr("[]"), want: "[]"},
		{name: "configured", tags: ormx.JSONArr(configuredTags), want: configuredTags},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newAlertMuteTestCtx(t)
			now := time.Now().Unix()
			m := &AlertMute{
				GroupId: 1, Cause: "maintenance", Btime: now, Etime: now + 3600,
				CreateBy: "creator", UpdateBy: "creator", Tags: tt.tags,
				DatasourceIdsJson: []int64{2}, SeveritiesJson: []int{2},
			}
			require.NoError(t, m.Add(c))

			check := func() {
				t.Helper()
				var storedTags sql.NullString
				require.NoError(t, DB(c).Raw("SELECT tags FROM alert_mute WHERE id = ?", m.Id).Row().Scan(&storedTags))
				require.True(t, storedTags.Valid, "tags must not be SQL NULL")
				require.Equal(t, tt.want, storedTags.String)
				got, err := AlertMuteGetById(c, m.Id)
				require.NoError(t, err)
				require.NotNil(t, got)
				require.Equal(t, tt.want, string(got.Tags))
				require.Equal(t, int64(1), got.GroupId)
				require.Equal(t, m.CreateAt, got.CreateAt)
				require.Equal(t, "creator", got.CreateBy)
				require.Empty(t, got.Note, "empty titles remain compatible")
				require.Equal(t, []int64{2}, got.DatasourceIdsJson)
				require.Equal(t, []int{2}, got.SeveritiesJson)
				encoded, err := json.Marshal(got)
				require.NoError(t, err)
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(encoded, &fields))
				require.JSONEq(t, tt.want, string(fields["tags"]))
			}
			check()

			// Start from a configured rule so clearing tags exercises full updates.
			require.NoError(t, DB(c).Model(m).Update("tags", configuredTags).Error)
			patch := *m
			patch.Tags = tt.tags
			patch.GroupId = 99
			patch.CreateBy = "replacement"
			patch.Etime = now + 7200
			patch.UpdateBy = "editor"
			require.NoError(t, m.Update(c, patch))
			check()
			got, err := AlertMuteGetById(c, m.Id)
			require.NoError(t, err)
			require.Equal(t, now+7200, got.Etime)
			require.Equal(t, "editor", got.UpdateBy)
		})
	}
}

func TestAlertMuteNullTagsReadLegacyRows(t *testing.T) {
	const configuredTags = `[{"key":"ident","func":"==","value":"web01"}]`
	tests := []struct {
		name string
		tags interface{}
		want string
	}{
		{name: "sql_null", want: "[]"},
		{name: "json_null", tags: "null", want: "[]"},
		{name: "json_null_with_whitespace", tags: " \nnull\t", want: "[]"},
		{name: "empty_array", tags: "[]", want: "[]"},
		{name: "configured", tags: configuredTags, want: configuredTags},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newAlertMuteTestCtx(t)
			now := time.Now().Unix()
			m := &AlertMute{
				GroupId: 1, Btime: now - 60, Etime: now + 3600,
				DatasourceIds: "[0]", PeriodicMutes: "[]", Tags: ormx.JSONArr("[]"),
			}
			insertMute(t, c, m)
			require.NoError(t, DB(c).Exec("UPDATE alert_mute SET tags = ? WHERE id = ?", tt.tags, m.Id).Error)
			got, err := AlertMuteGetById(c, m.Id)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tt.want, string(got.Tags))

			lists := []struct {
				name string
				read func() (interface{}, error)
			}{
				{"filtered", func() (interface{}, error) { return AlertMuteGets(c, nil, 1, -1, -1, "") }},
				{"group", func() (interface{}, error) { return AlertMuteGetsByBG(c, 1) }},
				{"groups", func() (interface{}, error) { return AlertMuteGetsByBGIds(c, []int64{1}) }},
				{"engine", func() (interface{}, error) { return AlertMuteGetsAll(c) }},
			}
			for _, list := range lists {
				t.Run(list.name, func(t *testing.T) {
					rows, err := list.read()
					require.NoError(t, err)
					encoded, err := json.Marshal(rows)
					require.NoError(t, err)
					var fields []map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(encoded, &fields))
					require.Len(t, fields, 1)
					require.JSONEq(t, tt.want, string(fields[0]["tags"]))
				})
			}
			// Read compatibility must not rewrite stored rows or their timestamps.
			var storedTags sql.NullString
			require.NoError(t, DB(c).Raw("SELECT tags FROM alert_mute WHERE id = ?", m.Id).Row().Scan(&storedTags))
			if tt.tags == nil {
				require.False(t, storedTags.Valid)
			} else {
				require.Equal(t, tt.tags, storedTags.String)
			}
		})
	}
}

func TestAlertMuteNullTagsRejectsInvalidConditions(t *testing.T) {
	for _, tags := range []string{`{}`, `"null"`, `[{`} {
		t.Run(tags, func(t *testing.T) {
			c := newAlertMuteTestCtx(t)
			m := &AlertMute{GroupId: 1, Btime: 1, Etime: 2, Tags: ormx.JSONArr(tags)}
			require.Error(t, m.Add(c))
			var count int64
			require.NoError(t, DB(c).Model(&AlertMute{}).Count(&count).Error)
			require.Zero(t, count, "invalid conditions must not become unrestricted mutes")
		})
	}
}
