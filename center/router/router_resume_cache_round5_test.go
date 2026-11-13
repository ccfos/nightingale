package router

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResumeEffectCache_ReturnsSavedResultUntilExpiry(t *testing.T) {
	rt := &Router{}
	server := targetReadRedis(t, rt)
	key := resumeEffectKey("chat-1", 1, "apply_rule", `{}`)
	rt.putResumeEffect(key, "applied")

	got, exists := rt.getResumeEffect(key)

	require.True(t, exists)
	require.Equal(t, "applied", got)
	require.Equal(t, resumeEffectTTL, server.TTL(key))
	server.FastForward(resumeEffectTTL)
	got, exists = rt.getResumeEffect(key)
	require.False(t, exists)
	require.Empty(t, got)
}

func TestResumeEffectCache_EmptyAndMissingResultsAreMisses(t *testing.T) {
	rt := &Router{}
	targetReadRedis(t, rt)
	rt.putResumeEffect("empty", "")

	for _, key := range []string{"empty", "missing"} {
		got, exists := rt.getResumeEffect(key)

		require.False(t, exists)
		require.Empty(t, got)
	}
}

func TestResumeEffectCache_UnavailableRedisPreservesSavedResult(t *testing.T) {
	rt := &Router{}
	server := targetReadRedis(t, rt)
	rt.putResumeEffect("effect", "original")
	server.SetError("ERR storage unavailable")

	rt.putResumeEffect("effect", "replacement")
	got, exists := rt.getResumeEffect("effect")

	require.False(t, exists)
	require.Empty(t, got)
	server.SetError("")
	got, exists = rt.getResumeEffect("effect")
	require.True(t, exists)
	require.Equal(t, "original", got)
}

func TestResumeEffectCache_WithoutRedisReportsMiss(t *testing.T) {
	rt := &Router{}

	rt.putResumeEffect("effect", "applied")
	got, exists := rt.getResumeEffect("effect")

	require.False(t, exists)
	require.Empty(t, got)
}

func TestHistoryBudgetFromContextLength_UsesPositiveTokenBudget(t *testing.T) {
	zero, negative, positive := 0, -100, 32000
	cases := []struct {
		name   string
		length *int
		want   int
	}{
		{name: "missing"},
		{name: "zero", length: &zero},
		{name: "negative", length: &negative},
		{name: "configured", length: &positive, want: 48000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := historyBudgetFromContextLength(tc.length)

			// The history allocation uses 3 bytes per token, reserving half for the current turn.
			require.Equal(t, tc.want, got)
		})
	}
}
