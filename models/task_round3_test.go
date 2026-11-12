package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskTplCleanFields_NormalizesExecutionFields(t *testing.T) {
	tpl := TaskTpl{Title: "restart", Script: "echo one\r\necho two", Pause: " 1， 2 ", Args: "a，b", Tags: "ops，linux"}

	err := tpl.CleanFields()

	require.NoError(t, err)
	require.Equal(t, 30, tpl.Timeout)
	require.Equal(t, "1,2", tpl.Pause)
	require.Equal(t, "a,b", tpl.Args)
	require.Equal(t, "ops,linux", tpl.Tags)
	require.Equal(t, "echo one\necho two", tpl.Script)
}

func TestTaskTplCleanFields_RejectsInvalidFields(t *testing.T) {
	cases := []struct {
		name    string
		change  func(*TaskTpl)
		message string
	}{
		{name: "batch", change: func(v *TaskTpl) { v.Batch = -1 }, message: "batch"},
		{name: "tolerance", change: func(v *TaskTpl) { v.Tolerance = -1 }, message: "tolerance"},
		{name: "negative timeout", change: func(v *TaskTpl) { v.Timeout = -1 }, message: "timeout"},
		{name: "long timeout", change: func(v *TaskTpl) { v.Timeout = 432001 }, message: "five days"},
		{name: "missing title", change: func(v *TaskTpl) { v.Title = "" }, message: "title"},
		{name: "unsafe title", change: func(v *TaskTpl) { v.Title = "<script>" }, message: "title"},
		{name: "missing script", change: func(v *TaskTpl) { v.Script = "" }, message: "script"},
		{name: "unsafe args", change: func(v *TaskTpl) { v.Args = "<script>" }, message: "args"},
		{name: "unsafe pause", change: func(v *TaskTpl) { v.Pause = "<script>" }, message: "pause"},
		{name: "unsafe tags", change: func(v *TaskTpl) { v.Tags = "<script>" }, message: "tags"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tpl := TaskTpl{Title: "restart", Script: "echo ok"}
			tc.change(&tpl)

			err := tpl.CleanFields()

			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestTaskFormVerify_RejectsInvalidExecutionRequests(t *testing.T) {
	cases := []struct {
		name    string
		change  func(*TaskForm)
		message string
	}{
		{name: "batch", change: func(v *TaskForm) { v.Batch = -1 }, message: "batch"},
		{name: "negative authorization", change: func(v *TaskForm) { v.AuthLevel = -1 }, message: "auth_level"},
		{name: "high authorization", change: func(v *TaskForm) { v.AuthLevel = 4 }, message: "auth_level"},
		{name: "tolerance", change: func(v *TaskForm) { v.Tolerance = -1 }, message: "tolerance"},
		{name: "negative timeout", change: func(v *TaskForm) { v.Timeout = -1 }, message: "timeout"},
		{name: "long timeout", change: func(v *TaskForm) { v.Timeout = 432001 }, message: "five days"},
		{name: "missing title", change: func(v *TaskForm) { v.Title = "" }, message: "title"},
		{name: "unsafe title", change: func(v *TaskForm) { v.Title = "<script>" }, message: "title"},
		{name: "missing script", change: func(v *TaskForm) { v.Script = "" }, message: "script"},
		{name: "unsafe args", change: func(v *TaskForm) { v.Args = "<script>" }, message: "args"},
		{name: "unsafe pause", change: func(v *TaskForm) { v.Pause = "<script>" }, message: "pause"},
		{name: "missing hosts", change: func(v *TaskForm) { v.Hosts = nil }, message: "hosts"},
		{name: "unsupported action", change: func(v *TaskForm) { v.Action = "stop" }, message: "action"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			form := TaskForm{Title: "restart", Script: "echo ok", Hosts: []string{"host-a"}, Action: "start"}
			tc.change(&form)

			err := form.Verify()

			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestTaskFormVerify_NormalizesSupportedRequests(t *testing.T) {
	for _, action := range []string{"start", "pause"} {
		t.Run(action, func(t *testing.T) {
			form := TaskForm{
				Title: "restart", Script: "echo one\r\necho two", Hosts: []string{"host-a"},
				Action: action, AuthLevel: 3, Args: "a，b", Pause: " 1， 2 ",
			}

			err := form.Verify()

			require.NoError(t, err)
			require.Equal(t, 30, form.Timeout)
			require.Equal(t, "1,2", form.Pause)
			require.Equal(t, "a,b", form.Args)
			require.Equal(t, "echo one\necho two", form.Script)
		})
	}
}

func TestTaskFormHandleFH_ReplacesPreviousFingerprint(t *testing.T) {
	form := TaskForm{Title: "restart"}

	form.HandleFH("first")
	require.Equal(t, "restart FH: first", form.Title)
	form.HandleFH("second")

	require.Equal(t, "restart FH: second", form.Title)
}

func TestTaskTplQueries_CombineFiltersAndDecodeTags(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Create(&[]TaskTpl{
		{Id: 1, GroupId: 7, Title: "alpha restart", Tags: "linux prod ", AuthLevel: 2, UpdateAt: 10},
		{Id: 2, GroupId: 7, Title: "beta restart", Tags: "linux prod ", AuthLevel: 2, UpdateAt: 20},
		{Id: 3, GroupId: 8, Title: "gamma restart", Tags: "linux prod ", AuthLevel: 2, UpdateAt: 30},
		{Id: 4, GroupId: 7, Title: "delta restart", Tags: "linux dev ", AuthLevel: 2, UpdateAt: 40},
		{Id: 5, GroupId: 7, Title: "epsilon restart", Tags: "linux prod ", AuthLevel: 1, UpdateAt: 50},
	}).Error)

	total, err := TaskTplTotal(c, []int64{7}, "restart prod", []int{2})
	require.NoError(t, err)
	rows, err := TaskTplGets(c, []int64{7}, "restart prod", []int{2}, 1, 1)

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 1)
	require.Equal(t, int64(2), rows[0].Id)
	require.Equal(t, []string{"linux", "prod"}, rows[0].TagsJSON)
	all, err := TaskTplTotal(c, nil, "", nil)
	require.NoError(t, err)
	require.Equal(t, int64(5), all)
}

func TestTaskTplSync_ReturnsAllTemplatesAndWatermark(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Create(&[]TaskTpl{{Id: 1, UpdateAt: 10}, {Id: 2, UpdateAt: 30}}).Error)

	stats, err := TaskTplStatistics(c)
	require.NoError(t, err)
	rows, err := TaskTplGetAll(c)

	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Total)
	require.Equal(t, int64(30), stats.LastUpdated)
	require.Len(t, rows, 2)
}

func TestTaskTplGetById_DecodesTagsAndReturnsNilForMissing(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Create(&TaskTpl{Id: 1, Title: "restart", Tags: "linux prod "}).Error)

	row, err := TaskTplGetById(c, 1)

	require.NoError(t, err)
	require.Equal(t, "restart", row.Title)
	require.Equal(t, []string{"linux", "prod"}, row.TagsJSON)
	missing, err := TaskTplGetById(c, 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestTaskRecordQueries_CombineFiltersBeforePagination(t *testing.T) {
	c := pipelineContext(t, &TaskRecord{})
	rows := []TaskRecord{
		{Id: 1, GroupId: 7, CreateAt: 100, CreateBy: "alice", Title: "restart api", AuthLevel: 2},
		{Id: 2, GroupId: 7, CreateAt: 200, CreateBy: "alice", Title: "restart api", AuthLevel: 2},
		{Id: 3, GroupId: 8, CreateAt: 300, CreateBy: "alice", Title: "restart api", AuthLevel: 2},
		{Id: 4, GroupId: 7, CreateAt: 400, CreateBy: "bob", Title: "restart api", AuthLevel: 2},
		{Id: 5, GroupId: 7, CreateAt: 500, CreateBy: "alice", Title: "restart db", AuthLevel: 2},
		{Id: 6, GroupId: 7, CreateAt: 600, CreateBy: "alice", Title: "restart api", AuthLevel: 1},
	}
	for i := range rows {
		require.NoError(t, rows[i].Add(c))
	}

	total, err := TaskRecordTotal(c, []int64{7}, 99, "alice", "api", []int{2})
	require.NoError(t, err)
	got, err := TaskRecordGets(c, []int64{7}, 99, "alice", "api", []int{2}, 1, 1)

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
	// The time boundary is strict, so the record exactly at 100 is excluded.
	total, err = TaskRecordTotal(c, []int64{7}, 100, "alice", "api", []int{2})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
}

func TestTaskRecordGetById_ReturnsPersistedRequestOrNil(t *testing.T) {
	c := pipelineContext(t, &TaskRecord{})
	row := TaskRecord{Title: "restart", Script: "echo ok", AuthLevel: 3, SystemCaller: "assistant"}
	require.NoError(t, row.Add(c))

	got, err := TaskRecordGetById(c, row.Id)

	require.NoError(t, err)
	require.Equal(t, row, *got)
	missing, err := TaskRecordGetById(c, row.Id+1)
	require.NoError(t, err)
	require.Nil(t, missing)
}
