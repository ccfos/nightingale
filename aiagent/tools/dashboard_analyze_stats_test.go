package tools

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// mkSeries builds a constant-step series starting at t0 from the given values.
func mkSeries(t0, step int64, vals []float64) ([]int64, []float64) {
	ts := make([]int64, len(vals))
	for i := range vals {
		ts[i] = t0 + int64(i)*step
	}
	return ts, vals
}

// steady returns n copies of base with tiny alternating noise so the series is
// NOT flat but has near-zero MAD-able variation.
func steady(n int, base float64) []float64 {
	vals := make([]float64, n)
	for i := range vals {
		noise := 0.0
		if i%2 == 1 {
			noise = base * 0.001
		}
		vals[i] = base + noise
	}
	return vals
}

func TestComputeSeriesStats_FlatAndFeatures(t *testing.T) {
	ts, vals := mkSeries(1000, 60, []float64{5, 5, 5, 5})
	s := computeSeriesStats(ts, vals)
	require.True(t, s.Flat, "identical values must be flagged flat")

	ts, vals = mkSeries(1000, 60, []float64{1, 9, 3, 2})
	s = computeSeriesStats(ts, vals)
	require.False(t, s.Flat)
	require.Equal(t, 9.0, s.Max)
	require.Equal(t, int64(1060), s.MaxTs, "MaxTs must point at the max sample")
	require.Equal(t, 2.0, s.Last)
	require.InDelta(t, 3.75, s.Avg, 1e-9)
}

func TestMadOutliers_FlagsLoneSpikeOnly(t *testing.T) {
	vals := steady(30, 20)
	vals[17] = 96 // lone wild point
	ts, _ := mkSeries(0, 60, vals)

	marks := madOutliers(ts, vals, madK)
	require.Len(t, marks, 1, "exactly the injected point must be flagged")
	require.Equal(t, int64(17*60), marks[0].Ts)
	require.Greater(t, marks[0].Pct, 100.0, "magnitude is far above the median level")

	// A clean steady series must not produce outliers.
	require.Empty(t, madOutliers(mkSeriesT(0, 60, steady(30, 20))), "steady series must be quiet")
}

// mkSeriesT adapts mkSeries for call sites that want both slices inline.
func mkSeriesT(t0, step int64, vals []float64) ([]int64, []float64, float64) {
	ts, v := mkSeries(t0, step, vals)
	return ts, v, madK
}

func TestJumpSpikes_DetectsStepChangeEdge(t *testing.T) {
	// 20 → 80 step change: exactly one big adjacent jump at the boundary.
	vals := append(steady(15, 20), steady(15, 80)...)
	ts, _ := mkSeries(0, 60, vals)

	marks := jumpSpikes(ts, vals, spikeK, 60)
	require.NotEmpty(t, marks, "the step edge must register as a jump")
	require.Equal(t, int64(15*60), marks[0].Ts, "jump lands on the first point of the new level")
	require.Greater(t, marks[0].Pct, 100.0)

	// Small relative wiggle (< spikeMinRelPct) must be suppressed even though
	// MAD of diffs is tiny.
	vals = steady(30, 1000)
	vals[10] = 1030 // +3% — big in MAD terms on a quiet series, small relatively
	ts, _ = mkSeries(0, 60, vals)
	require.Empty(t, jumpSpikes(ts, vals, spikeK, 60), "sub-threshold relative jumps must be suppressed")
}

// TestJumpSpikes_GapBoundaryNotASpike: samplePairsToSlices compacts NaN/
// staleness gaps out of the slices, so array-adjacent points can be far apart
// in time. The level difference across such a gap (exporter down 14:00-14:20,
// back at a new level) is recovery, not an adjacent-point jump — without the
// gap guard it fires as a false 突变 that the periodic downgrade can never
// rescue (a current-window-only gap has no prev counterpart).
func TestJumpSpikes_GapBoundaryNotASpike(t *testing.T) {
	// steady 20, a 10-step hole, then steady 80.
	ts1, vals1 := mkSeries(0, 60, steady(15, 20))
	ts2, vals2 := mkSeries(25*60, 60, steady(15, 80))
	ts := append(ts1, ts2...)
	vals := append(vals1, vals2...)
	require.Empty(t, jumpSpikes(ts, vals, spikeK, 60), "a diff across a staleness gap is not a spike")

	// The same 20→80 edge with contiguous points must still fire.
	valsC := append(steady(15, 20), steady(15, 80)...)
	tsC, _ := mkSeries(0, 60, valsC)
	require.NotEmpty(t, jumpSpikes(tsC, valsC, spikeK, 60), "contiguous level change must still register")

	// step<=0 disables the guard (defensive default for direct callers).
	require.NotEmpty(t, jumpSpikes(ts, vals, spikeK, 0), "guard off → old adjacency behavior")
}

func TestTrendChangePct_GradualRamp(t *testing.T) {
	// Ramp 10 → 30: last-quarter mean is far above first-quarter mean.
	vals := make([]float64, 40)
	for i := range vals {
		vals[i] = 10 + float64(i)*0.5
	}
	pct := trendChangePct(vals)
	require.Greater(t, pct, trendThresholdPct, "a clear ramp must exceed the trend threshold")

	require.InDelta(t, 0, trendChangePct(steady(40, 50)), 1.0, "steady series has ~no trend")
}

func TestAnalyzeSeries_FlatShortCircuits(t *testing.T) {
	ts, vals := mkSeries(0, 60, []float64{7, 7, 7, 7, 7, 7, 7, 7})
	f := analyzeSeries(ts, vals, nil, nil, 86400, 60)
	require.True(t, f.Stats.Flat)
	require.False(t, f.Suspicious)
	require.Empty(t, f.Outliers)
}

func TestAnalyzeSeries_SpikeMakesSuspicious(t *testing.T) {
	vals := steady(40, 20)
	vals[25] = 90
	ts, _ := mkSeries(0, 60, vals)

	f := analyzeSeries(ts, vals, nil, nil, 86400, 60)
	require.True(t, f.Suspicious)
	require.Greater(t, f.Score, 1000.0, "at least one detector fired")
	require.Nil(t, f.YoY, "no comparison window → no YoY block")
}

func TestAnalyzeSeries_YoYLevelShift(t *testing.T) {
	// Current window runs at 3x yesterday's level (no local spikes).
	curTs, curVals := mkSeries(86400, 60, steady(40, 60))
	prevTs, prevVals := mkSeries(0, 60, steady(40, 20))

	f := analyzeSeries(curTs, curVals, prevTs, prevVals, 86400, 60)
	require.NotNil(t, f.YoY)
	require.True(t, f.YoY.Hit, "a 200%% avg shift must trip the YoY detector")
	require.InDelta(t, 200, f.YoY.AvgPct, 5)
	require.True(t, f.Suspicious)
}

func TestAnalyzeSeries_PeriodicSpikeDowngraded(t *testing.T) {
	// Same spike at the same offset with the same magnitude in both windows —
	// the daily-cron pattern. Must NOT stay suspicious.
	mk := func(t0 int64) ([]int64, []float64) {
		vals := steady(40, 20)
		vals[25] = 90
		return mkSeries(t0, 60, vals)
	}
	curTs, curVals := mk(86400)
	prevTs, prevVals := mk(0)

	f := analyzeSeries(curTs, curVals, prevTs, prevVals, 86400, 60)
	require.True(t, f.Periodic, "recurring anomaly at the same offset must be recognised")
	require.False(t, f.Suspicious, "periodic recurrence without level shift is downgraded")

	// Same spike but yesterday is clean → genuinely new anomaly, stays suspicious.
	cleanPrevTs, cleanPrevVals := mkSeries(0, 60, steady(40, 20))
	f = analyzeSeries(curTs, curVals, cleanPrevTs, cleanPrevVals, 86400, 60)
	require.False(t, f.Periodic)
	require.True(t, f.Suspicious)
}

func TestAnalyzeSeries_WorsenedPeriodicSpikeStaysSuspicious(t *testing.T) {
	mk := func(t0 int64, peak float64) ([]int64, []float64) {
		vals := steady(40, 20)
		vals[25] = peak
		return mkSeries(t0, 60, vals)
	}

	// Today's spike is ~2.6x yesterday's magnitude at the same offset — a
	// worsening regression, not a routine recurrence: must NOT be downgraded.
	curTs, curVals := mk(86400, 200) // ≈ +900% vs the 20 baseline
	prevTs, prevVals := mk(0, 90)    // ≈ +350%
	f := analyzeSeries(curTs, curVals, prevTs, prevVals, 86400, 60)
	require.False(t, f.Periodic, "a spike far beyond yesterday's must not count as recurring")
	require.True(t, f.Suspicious)

	// Mildly grown recurrence (~1.4x) stays inside the tolerance band and is
	// still downgraded as periodic.
	curTs, curVals = mk(86400, 120) // ≈ +500%
	prevTs, prevVals = mk(0, 90)    // ≈ +350%, ratio ≈ 0.7 ≥ periodicMagLo
	f = analyzeSeries(curTs, curVals, prevTs, prevVals, 86400, 60)
	require.True(t, f.Periodic)
	require.False(t, f.Suspicious)
}

func TestAnalyzeSeries_OppositeDirectionNotPeriodic(t *testing.T) {
	// Yesterday SPIKED at offset 25; today it DIPS at the same offset with a
	// comparable |magnitude|. A recurring pattern repeats in the same
	// direction — a service that used to spike but now crashes is news, and
	// must NOT be downgraded as "periodic".
	mk := func(t0 int64, point float64) ([]int64, []float64) {
		vals := steady(40, 60)
		vals[25] = point
		return mkSeries(t0, 60, vals)
	}
	curTs, curVals := mk(86400, 10) // dip:   ≈ -83% vs the 60 baseline
	prevTs, prevVals := mk(0, 110)  // spike: ≈ +83%
	f := analyzeSeries(curTs, curVals, prevTs, prevVals, 86400, 60)
	require.False(t, f.Periodic, "a dip must not match yesterday's spike at the same offset")
	require.True(t, f.Suspicious)
}

func TestAnalyzeSeries_ZeroBaselineScoreClamped(t *testing.T) {
	// A 0→x onset hits relPct's zero-baseline cap (99900%). Its score must be
	// clamped (magTerm) so it cannot permanently outrank a genuinely large
	// real incident on a nonzero baseline.
	onsetVals := steady(40, 0)
	for i := 30; i < 40; i++ {
		onsetVals[i] = 5
	}
	onsetTs, _ := mkSeries(86400, 60, onsetVals)
	onset := analyzeSeries(onsetTs, onsetVals, nil, nil, 86400, 60)

	require.LessOrEqual(t, onset.Score, float64(4)*1000+4*magTermCap,
		"capped-percent artifacts must not blow the score past the clamp")
}

func TestAnalyzeSeries_FlatStillComparedToYesterday(t *testing.T) {
	// Exporter keeps reporting but the value pegs at 0 all window while
	// yesterday ran at a healthy level — the flatline outage signature must
	// stay suspicious, not hide in the flat bucket.
	curTs, curVals := mkSeries(86400, 60, make([]float64, 40))
	prevTs, prevVals := mkSeries(0, 60, steady(40, 100))
	f := analyzeSeries(curTs, curVals, prevTs, prevVals, 86400, 60)
	require.True(t, f.Stats.Flat)
	require.NotNil(t, f.YoY, "flat series must still get the YoY comparison")
	require.True(t, f.YoY.Hit)
	require.True(t, f.Suspicious)
	require.Empty(t, f.Outliers, "point detectors stay off for flat series")

	// Flat today at the same level as yesterday → genuinely boring.
	samePrevTs, samePrevVals := mkSeries(0, 60, make([]float64, 40))
	f = analyzeSeries(curTs, curVals, samePrevTs, samePrevVals, 86400, 60)
	require.True(t, f.Stats.Flat)
	require.False(t, f.Suspicious)
}

func TestDownsampleForDisplay_KeepsAnomalyPoints(t *testing.T) {
	vals := steady(300, 20)
	vals[137] = 95
	ts, _ := mkSeries(0, 60, vals)
	marks := []pointMark{{Ts: 137 * 60, Val: 95}}

	outTs, outVals := downsampleForDisplay(ts, vals, marks, 30)
	require.LessOrEqual(t, len(outTs), 30+len(marks)+1, "stays near the requested budget")
	found := false
	for i, tsv := range outTs {
		if tsv == 137*60 {
			require.Equal(t, 95.0, outVals[i])
			found = true
		}
	}
	require.True(t, found, "the anomalous point must survive downsampling")
	require.Equal(t, ts[len(ts)-1], outTs[len(outTs)-1], "last point must be kept")

	// Short series pass through untouched.
	sTs, sVals := mkSeries(0, 60, steady(10, 5))
	oTs, oVals := downsampleForDisplay(sTs, sVals, nil, 30)
	require.Equal(t, sTs, oTs)
	require.Equal(t, sVals, oVals)
}

func TestRelPct_ZeroBaseGuard(t *testing.T) {
	require.Equal(t, 99900.0, relPct(10, 0), "zero base must cap, not Inf")
	require.False(t, math.IsInf(relPct(-10, 0), -1))
}

// TestAnalyzeSeries_NearZeroQuietSeriesNotSuspicious is the regression for the
// near-zero false-positive class: with the majority of points identical AND
// the level ~0 (quiet error counters, idle queues), madDenominator is
// degenerate, and before the minMadScale guard every micro wiggle was flagged
// at the capped ±99900%, outranking real incidents — while the identical
// wiggle at a real level (45) was correctly quiet.
func TestAnalyzeSeries_NearZeroQuietSeriesNotSuspicious(t *testing.T) {
	vals := make([]float64, 30) // mostly-zero, occasional micro blips
	vals[5], vals[12], vals[21] = 0.002, 0.001, 0.003
	ts, _ := mkSeries(0, 60, vals)

	require.Empty(t, madOutliers(ts, vals, madK), "micro blips on a near-zero series are noise, not outliers")
	require.Empty(t, jumpSpikes(ts, vals, spikeK, 60), "micro jumps on a near-zero series are noise, not spikes")

	f := analyzeSeries(ts, vals, nil, nil, 86400, 60)
	require.False(t, f.Suspicious, "a quiet near-zero series must not rank as suspicious")

	// Positive control: the degenerate-scale guard must not change behavior
	// at a non-zero level, where the |med|*5% fallback is meaningful.
	shifted := make([]float64, 30)
	for i := range shifted {
		shifted[i] = 45
	}
	shifted[5], shifted[12], shifted[21] = 45.002, 45.001, 45.003
	fs := analyzeSeries(ts, shifted, nil, nil, 86400, 60)
	require.False(t, fs.Suspicious)
}

// TestAnalyzeSeries_NearZeroLevelShiftStillCaught documents the division of
// labor: the point detectors skip degenerate near-zero series, but a sustained
// zero→nonzero shift (errors start flowing) is still caught by the trend
// detector, which compares window quarters rather than points.
func TestAnalyzeSeries_NearZeroLevelShiftStillCaught(t *testing.T) {
	vals := make([]float64, 32)
	for i := 24; i < 32; i++ {
		vals[i] = 5 // last quarter: errors start flowing
	}
	ts, _ := mkSeries(0, 60, vals)
	f := analyzeSeries(ts, vals, nil, nil, 86400, 60)
	require.True(t, f.TrendHit, "a sustained zero→nonzero shift must fire the trend detector")
	require.True(t, f.Suspicious)
}
