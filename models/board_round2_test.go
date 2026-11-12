package models

import (
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
)

func TestBoardVerify_RejectsInvalidNames(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{name: "", want: "Name is blank"},
		{name: "<script>", want: "Name has invalid characters"},
		{name: "Production overview"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			board := Board{Name: tc.name}

			err := board.Verify()

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.want)
			}
		})
	}
}

func TestBoardClone_CopiesContentWithNewOwnership(t *testing.T) {
	for _, ident := range []string{"", "original"} {
		t.Run(ident, func(t *testing.T) {
			board := Board{Id: 42, Name: "CPU", Ident: ident, Tags: "prod", Note: "host usage", GroupId: 1, Public: 1}

			clone := board.Clone("operator", 7, "copy")

			require.Equal(t, "CPU copy", clone.Name)
			require.Equal(t, board.Tags, clone.Tags)
			require.Equal(t, board.Note, clone.Note)
			require.Equal(t, int64(7), clone.GroupId)
			require.Equal(t, "operator", clone.CreateBy)
			require.Equal(t, "operator", clone.UpdateBy)
			require.Zero(t, clone.Id)
			require.Zero(t, clone.Public)
			if ident == "" {
				require.Empty(t, clone.Ident)
			} else {
				require.NotEmpty(t, clone.Ident)
				require.NotEqual(t, ident, clone.Ident)
			}
			require.Equal(t, "CPU", board.Name)
			require.Equal(t, "CPU", board.Clone("operator", 7, "").Name)
		})
	}
}

func TestBoardAdd_EnforcesIdentAndGroupNameUniqueness(t *testing.T) {
	c := pipelineContext(t, &Board{})
	original := Board{Name: "CPU", Ident: "cpu", GroupId: 1}
	require.NoError(t, original.Add(c))
	cases := []struct {
		name  string
		board Board
		want  string
	}{
		{
			name: "duplicate ident across groups", board: Board{Name: "Other", Ident: "cpu", GroupId: 2},
			want: "Ident duplicate",
		},
		{name: "duplicate group name", board: Board{Name: "CPU", GroupId: 1}, want: "Name duplicate"},
		{name: "same name in another group", board: Board{Name: "CPU", GroupId: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.board.Add(c)

			if tc.want == "" {
				require.NoError(t, err)
				require.Positive(t, tc.board.Id)
			} else {
				require.EqualError(t, err, tc.want)
			}
		})
	}
}

func TestBoardCanRenameIdent_AllowsOwnIdentAndRejectsOtherBoardIdent(t *testing.T) {
	c := pipelineContext(t, &Board{})
	original := Board{Id: 1, Name: "CPU", Ident: "cpu"}
	require.NoError(t, c.DB.Create(&original).Error)

	for _, ident := range []string{"", "cpu", "unused"} {
		allowed, err := original.CanRenameIdent(c, ident)
		require.NoError(t, err)
		require.True(t, allowed)
	}
	other := Board{Id: 99}
	allowed, err := other.CanRenameIdent(c, "cpu")
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestBoardAtomicAdd_RollsBackWhenPayloadCannotBeSaved(t *testing.T) {
	c := pipelineContext(t, &Board{})
	board := Board{Name: "CPU", GroupId: 1}

	err := board.AtomicAdd(c, `{"panels":[]}`)

	require.Error(t, err)
	count, err := BoardCount(c, "name = ?", "CPU")
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestBoardPayloadSave_ReplacesContentAndDetailReadIncludesIt(t *testing.T) {
	c := pipelineContext(t, &Board{}, &BoardPayload{})
	board := Board{Name: "CPU", GroupId: 1}
	require.NoError(t, board.AtomicAdd(c, `{"title":"old"}`))

	require.NoError(t, BoardPayloadSave(c, board.Id, `{"title":"new"}`))
	loaded, err := BoardGet(c, "id = ?", board.Id)

	require.NoError(t, err)
	require.Equal(t, `{"title":"new"}`, loaded.Configs)
	metadata, err := BoardGetByID(c, board.Id)
	require.NoError(t, err)
	require.Empty(t, metadata.Configs)
	payloads, err := BoardPayloadGets(c, []int64{board.Id, 999})
	require.NoError(t, err)
	require.Len(t, payloads, 1)
	require.Equal(t, board.Id, payloads[0].Id)
	_, err = BoardPayloadGets(c, nil)
	require.EqualError(t, err, "empty ids")
	missing, err := BoardGet(c, "id = ?", 999)
	require.NoError(t, err)
	require.Nil(t, missing)
	missing, err = BoardGetByID(c, 999)
	require.NoError(t, err)
	require.Nil(t, missing)
	payload, err := BoardPayloadGet(c, 999)
	require.NoError(t, err)
	require.Empty(t, payload)
}

func TestBoardDel_DeletesOnlyItsPayloadAndMetadata(t *testing.T) {
	c := pipelineContext(t, &Board{}, &BoardPayload{})
	board := Board{Name: "CPU", GroupId: 1}
	other := Board{Name: "Memory", GroupId: 1}
	require.NoError(t, board.AtomicAdd(c, "cpu payload"))
	require.NoError(t, other.AtomicAdd(c, "memory payload"))

	require.NoError(t, board.Del(c))

	exists, err := BoardExists(c, "id = ?", board.Id)
	require.NoError(t, err)
	require.False(t, exists)
	payload, err := BoardPayloadGet(c, board.Id)
	require.NoError(t, err)
	require.Empty(t, payload)
	payload, err = BoardPayloadGet(c, other.Id)
	require.NoError(t, err)
	require.Equal(t, "memory payload", payload)
}

func TestBoardQueries_CombineGroupPositiveAndNegativeTerms(t *testing.T) {
	c := pipelineContext(t, &Board{})
	require.NoError(t, c.DB.Create(&[]Board{
		{Id: 1, Name: "CPU alpha", GroupId: 7, Tags: "prod"},
		{Id: 2, Name: "CPU beta", GroupId: 7, Tags: "prod legacy"},
		{Id: 3, Name: "CPU gamma", GroupId: 8, Tags: "prod"},
		{Id: 4, Name: "Memory", GroupId: 7, Tags: "prod"},
	}).Error)
	cases := []struct {
		name string
		get  func(*ctx.Context) ([]Board, error)
	}{
		{name: "one group", get: func(c *ctx.Context) ([]Board, error) {
			return BoardGetsByGroupId(c, 7, "CPU prod -legacy")
		}},
		{name: "group list", get: func(c *ctx.Context) ([]Board, error) {
			return BoardGetsByBGIds(c, []int64{7}, "CPU prod -legacy")
		}},
		{name: "explicit predicate", get: func(c *ctx.Context) ([]Board, error) {
			return BoardGets(c, "CPU prod -legacy", "group_id = ?", 7)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			boards, err := tc.get(c)

			require.NoError(t, err)
			require.Len(t, boards, 1)
			require.Equal(t, int64(1), boards[0].Id)
		})
	}
	boards, err := BoardGets(c, "", "")
	require.NoError(t, err)
	require.Len(t, boards, 4)
	require.Equal(t, "CPU alpha", boards[0].Name)
}

func TestBoardSetHide_ReplacesBuiltinSelection(t *testing.T) {
	c := pipelineContext(t, &Board{})
	require.NoError(t, c.DB.Create(&[]Board{
		{Id: 1, Name: "old hidden", BuiltIn: 1, Hide: 1},
		{Id: 2, Name: "new hidden", BuiltIn: 1},
		{Id: 3, Name: "custom", Hide: 1},
	}).Error)

	require.NoError(t, BoardSetHide(c, []int64{2, 3}))

	for id, want := range map[int64]int{1: 0, 2: 1, 3: 1} {
		board, err := BoardGetByID(c, id)
		require.NoError(t, err)
		require.Equal(t, want, board.Hide)
	}
}

func TestBoardGetsByBids_ResolvesGroupsAndOmitsOrphans(t *testing.T) {
	c := pipelineContext(t, &Board{}, &BusiGroup{})
	require.NoError(t, c.DB.Create(&BusiGroup{Id: 7, Name: "Operations"}).Error)
	require.NoError(t, c.DB.Create(&[]Board{
		{Id: 1, Name: "CPU", GroupId: 7},
		{Id: 2, Name: "orphan", GroupId: 99},
	}).Error)

	boards, err := BoardGetsByBids(c, []int64{1, 2})

	require.NoError(t, err)
	require.Equal(t, []map[string]interface{}{{
		"busi_group_name": "Operations", "busi_group_id": int64(7), "board_id": int64(1), "board_name": "CPU",
	}}, boards)
}

func TestBoardBusigroupUpdate_ReplacesOnlySelectedBoardMemberships(t *testing.T) {
	c := pipelineContext(t, &BoardBusigroup{})
	require.NoError(t, BoardBusigroupAdd(c.DB, 1, []int64{7, 8}))
	require.NoError(t, BoardBusigroupAdd(c.DB, 2, []int64{7}))

	require.NoError(t, BoardBusigroupUpdate(c, 1, []int64{9}))

	allowed, err := BoardBusigroupCheck(c, 1, []int64{7, 8})
	require.NoError(t, err)
	require.False(t, allowed)
	allowed, err = BoardBusigroupCheck(c, 1, []int64{9})
	require.NoError(t, err)
	require.True(t, allowed)
	ids, err := BoardIdsByBusiGroupIds(c, []int64{7, 9})
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{1, 2}, ids)
	require.NoError(t, BoardBusigroupUpdate(c, 1, nil))
	members, err := BoardBusigroupGets(c)
	require.NoError(t, err)
	require.Equal(t, []BoardBusigroup{{BoardId: 2, BusiGroupId: 7}}, members)
	require.NoError(t, BoardBusigroupDelByBoardId(c, 2))
	members, err = BoardBusigroupGets(c)
	require.NoError(t, err)
	require.Empty(t, members)
}
