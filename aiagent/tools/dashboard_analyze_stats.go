package tools

import (
	"math"
	"sort"
)

// ============================================================================
// Statistical pre-screening for get_dashboard_data.
//
// Design (docs/specs/2026-06-07-analyze-dashboard-skill-design.md): anomaly
// DETECTION runs server-side with deterministic statistics; the LLM only sees
// the screened findings and does correlation / explanation / drill-down. This
// mirrors the industry split (Grafana Sift's DBSCAN+MAD, Datadog Watchdog) —
// LLMs scanning raw points score F1 0.13-0.22 (SigLLM, arXiv 2405.14755), so
// we never ask the model to do the detecting.
//
// Everything in this file is a pure function over parallel slices
// (ts []int64, vals []float64) so it unit-tests without any fixture.
// ============================================================================

// Detection thresholds. Centralised so tests and docs reference one place.
const (
	madK              = 3.0  // modified z-score cutoff for point outliers
	spikeK            = 3.0  // MAD-of-diffs cutoff for adjacent-point jumps
	trendThresholdPct = 30.0 // first-quarter vs last-quarter mean change
	yoyThresholdPct   = 50.0 // avg change vs the shifted comparison window
	// spikeMinRelPct suppresses MAD-triggered "jumps" that are numerically
	// tiny relative to the series level (flat-ish series make MAD≈0, where
	// any noise would otherwise fire).
	spikeMinRelPct = 10.0
	// periodicMagLo/Hi bound the prev/cur magnitude ratio for a current
	// anomaly to count as "recurring" in the previous window (same offset
	// ±1 step). Used to downgrade daily-cron style spikes (Watchdog-lite).
	// Deliberately asymmetric: today may be up to 2× smaller than yesterday
	// (a fading recurrence is routine), but at most ~1.5× larger (lo=0.67) —
	// a spike that materially exceeds yesterday's is a worsening, not a
	// routine recurrence, and must stay suspicious.
	periodicMagLo = 0.67
	periodicMagHi = 2.0
	// minMadScale is the floor below which madDenominator's result counts as
	// degenerate: the majority of points are identical AND the level is ~0
	// (quiet error counters, idle queue depths, low-traffic rates). With no
	// meaningful scale, any nonzero wiggle would register as an outlier/spike
	// at the capped ±99900% and outrank real incidents, so the point
	// detectors skip such series; zero→nonzero level shifts are still caught
	// by trend/YoY, which compare windows rather than points.
	minMadScale = 1e-9
	// relPctCap bounds relPct against zero-ish baselines. A value AT the cap
	// is an artifact meaning "from ≈0", not a real percentage — renderers show
	// it qualitatively and the score clamps it (see magTerm).
	relPctCap = 99900.0
	// magTermCap bounds a single detector's magnitude contribution to the
	// suspicion score: an uncapped zero-baseline artifact (99900) would let a
	// trivial 0→x counter blip permanently outrank a real large incident.
	magTermCap = 2000.0
)

// seriesStats are the always-computed features of one curve.
type seriesStats struct {
	Avg, Min, Max, Last float64
	MaxTs               int64 // unix sec of the max point
	Flat                bool  // every value identical (within float noise)
	N                   int
}

// pointMark is one detected anomalous point (MAD outlier or jump spike).
type pointMark struct {
	Ts  int64
	Val float64
	Pct float64 // magnitude in percent (vs median for outliers, vs previous value for spikes)
}

// yoyCompare holds the comparison against the time-shifted window.
type yoyCompare struct {
	PrevAvg float64
	AvgPct  float64 // (curAvg - prevAvg) / |prevAvg| * 100
	MaxPct  float64
	Hit     bool
}

// seriesFindings aggregates all detections for one curve.
type seriesFindings struct {
	Stats      seriesStats
	Outliers   []pointMark
	Spikes     []pointMark
	TrendPct   float64
	TrendHit   bool
	YoY        *yoyCompare // nil when the shifted window returned no data
	Periodic   bool        // anomalies recur in the shifted window → likely cron/seasonal
	Score      float64     // suspicion ranking key (higher = more suspicious)
	Suspicious bool
}

func computeSeriesStats(ts []int64, vals []float64) seriesStats {
	s := seriesStats{N: len(vals)}
	if len(vals) == 0 {
		return s
	}
	s.Min, s.Max, s.MaxTs = vals[0], vals[0], ts[0]
	sum := 0.0
	flat := true
	for i, v := range vals {
		sum += v
		if v < s.Min {
			s.Min = v
		}
		if v > s.Max {
			s.Max = v
			s.MaxTs = ts[i]
		}
		if v != vals[0] {
			flat = false
		}
	}
	s.Avg = sum / float64(len(vals))
	s.Last = vals[len(vals)-1]
	s.Flat = flat
	return s
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	c := append([]float64(nil), vals...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

// mad returns the Median Absolute Deviation around med.
func mad(vals []float64, med float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	devs := make([]float64, len(vals))
	for i, v := range vals {
		devs[i] = math.Abs(v - med)
	}
	return median(devs)
}

// madDenominator converts MAD into a robust σ-equivalent scale; when MAD is 0
// (the majority of points identical) it falls back to a fraction of the median
// level so a lone wild point still registers while float noise does not. When
// the median is itself ~0 that fallback degenerates too — the result lands
// below minMadScale and callers must skip point detection rather than divide
// by a meaningless scale.
func madDenominator(madVal, med float64) float64 {
	d := madVal * 1.4826
	if d == 0 {
		d = math.Abs(med) * 0.05
	}
	return d
}

// relPct is a zero-guarded percent of delta against base.
func relPct(delta, base float64) float64 {
	denom := math.Abs(base)
	if denom < 1e-9 {
		denom = 1e-9
	}
	pct := delta / denom * 100
	// Cap so a zero-ish baseline doesn't print absurd percentages.
	if pct > relPctCap {
		pct = relPctCap
	}
	if pct < -relPctCap {
		pct = -relPctCap
	}
	return pct
}

// pctCapped reports whether p sits at relPct's zero-baseline cap — an
// artifact meaning "from ≈0", not a real percentage.
func pctCapped(p float64) bool {
	return math.Abs(p) >= relPctCap
}

// magTerm is one detector's clamped magnitude contribution to the score.
func magTerm(p float64) float64 {
	return math.Min(p, magTermCap)
}

// madOutliers flags points whose modified z-score exceeds k.
func madOutliers(ts []int64, vals []float64, k float64) []pointMark {
	if len(vals) < 4 {
		return nil
	}
	med := median(vals)
	denom := madDenominator(mad(vals, med), med)
	if denom < minMadScale {
		return nil
	}
	var marks []pointMark
	for i, v := range vals {
		if math.Abs(v-med)/denom > k {
			marks = append(marks, pointMark{Ts: ts[i], Val: v, Pct: relPct(v-med, med)})
		}
	}
	return marks
}

// jumpSpikes flags adjacent-point jumps that are large both in MAD-of-diffs
// terms and relative to the series level. step is the query resolution:
// samplePairsToSlices compacts NaN/staleness gaps out of the slices, so
// array-adjacent points can be far apart in time — the level difference across
// such a gap (scrape outage, exporter restart) is recovery, not a jump, and
// must neither fire as a spike nor pollute the MAD-of-diffs baseline. Only
// pairs within ~1.5 steps participate; step <= 0 disables the guard.
func jumpSpikes(ts []int64, vals []float64, k float64, step int64) []pointMark {
	if len(vals) < 5 {
		return nil
	}
	maxGap := step + step/2
	idxs := make([]int, 0, len(vals)-1) // idxs[j] = i ⇔ diffs[j] = vals[i] - vals[i-1]
	diffs := make([]float64, 0, len(vals)-1)
	for i := 1; i < len(vals); i++ {
		if maxGap > 0 && ts[i]-ts[i-1] > maxGap {
			continue
		}
		idxs = append(idxs, i)
		diffs = append(diffs, vals[i]-vals[i-1])
	}
	if len(diffs) < 4 { // mirror the len(vals) < 5 floor after gap filtering
		return nil
	}
	med := median(diffs)
	denom := madDenominator(mad(diffs, med), median(vals))
	if denom < minMadScale {
		return nil
	}
	var marks []pointMark
	for j, d := range diffs {
		if math.Abs(d-med)/denom <= k {
			continue
		}
		i := idxs[j]
		pct := relPct(d, vals[i-1]) // jump vs previous value
		if math.Abs(pct) < spikeMinRelPct {
			continue
		}
		marks = append(marks, pointMark{Ts: ts[i], Val: vals[i], Pct: pct})
	}
	return marks
}

// quarterMeans returns the means of the first and last quarter of the window —
// the trend detector's two anchor levels; ok is false when the window is too
// short to split.
func quarterMeans(vals []float64) (first, last float64, ok bool) {
	n := len(vals)
	if n < 8 {
		return 0, 0, false
	}
	q := n / 4
	for i := 0; i < q; i++ {
		first += vals[i]
		last += vals[n-q+i]
	}
	return first / float64(q), last / float64(q), true
}

// trendChangePct compares the mean of the last quarter of the window against
// the first quarter. Returns the signed percent change.
func trendChangePct(vals []float64) float64 {
	first, last, ok := quarterMeans(vals)
	if !ok {
		return 0
	}
	return relPct(last-first, first)
}

// anomalousPoints is the union of outliers and spikes, used by the periodicity
// check (a daily cron spike fires BOTH detectors, so matching only one of them
// would fail to recognise the recurrence).
func anomalousPoints(ts []int64, vals []float64, step int64) []pointMark {
	marks := append([]pointMark(nil), madOutliers(ts, vals, madK)...)
	marks = append(marks, jumpSpikes(ts, vals, spikeK, step)...)
	sort.Slice(marks, func(i, j int) bool { return marks[i].Ts < marks[j].Ts })
	return marks
}

// allRecurInPrev reports whether EVERY current anomalous point has a
// counterpart anomaly in the shifted window at the same offset (±tolerance)
// with comparable magnitude. One unmatched anomaly = not periodic.
func allRecurInPrev(cur, prev []pointMark, shift, tolerance int64) bool {
	if len(cur) == 0 || len(prev) == 0 {
		return false
	}
	for _, c := range cur {
		want := c.Ts - shift
		matched := false
		for _, p := range prev {
			if p.Ts < want-tolerance || p.Ts > want+tolerance {
				continue
			}
			// A recurring pattern repeats in the SAME direction — today's dip
			// must not be downgraded because yesterday SPIKED at this offset
			// (a service that used to spike but now crashes is news, not cron).
			if math.Signbit(p.Pct) != math.Signbit(c.Pct) {
				continue
			}
			ratio := math.Abs(p.Pct) / math.Max(math.Abs(c.Pct), 1e-9)
			if ratio >= periodicMagLo && ratio <= periodicMagHi {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// compareWindows builds the YoY block: the current window's stats against the
// shifted comparison window. Returns nil when there is no comparison data.
func compareWindows(cur seriesStats, prevTs []int64, prevVals []float64) *yoyCompare {
	if len(prevVals) == 0 {
		return nil
	}
	prevStats := computeSeriesStats(prevTs, prevVals)
	yoy := &yoyCompare{
		PrevAvg: prevStats.Avg,
		AvgPct:  relPct(cur.Avg-prevStats.Avg, prevStats.Avg),
		MaxPct:  relPct(cur.Max-prevStats.Max, prevStats.Max),
	}
	yoy.Hit = math.Abs(yoy.AvgPct) >= yoyThresholdPct
	return yoy
}

// analyzeSeries runs the full screening for one curve. prevTs/prevVals carry
// the shifted comparison window (may be empty), shift its offset in seconds,
// step the query resolution (tolerance for periodic point matching).
func analyzeSeries(ts []int64, vals []float64, prevTs []int64, prevVals []float64, shift, step int64) seriesFindings {
	f := seriesFindings{Stats: computeSeriesStats(ts, vals)}
	if f.Stats.N == 0 {
		return f
	}
	if f.Stats.Flat {
		// Flat series skip the point/trend detectors (MAD≈0 would turn any
		// noise into outliers) but NOT the YoY comparison: an exporter that
		// still reports while its value pegs constant (qps stuck at 0 after a
		// crash) is a strong failure signal precisely because yesterday's
		// level differed — it must not hide in the harmless "平直" bucket.
		f.YoY = compareWindows(f.Stats, prevTs, prevVals)
		if f.YoY != nil && f.YoY.Hit {
			f.Score = 1000 + magTerm(math.Abs(f.YoY.AvgPct))
			f.Suspicious = true
		}
		return f
	}

	f.Outliers = madOutliers(ts, vals, madK)
	f.Spikes = jumpSpikes(ts, vals, spikeK, step)
	f.TrendPct = trendChangePct(vals)
	f.TrendHit = math.Abs(f.TrendPct) >= trendThresholdPct
	// The percent trend shares the point detectors' near-zero degeneracy
	// (minMadScale): on a mostly-zero series a single micro blip landing in
	// the first or last quarter drags that quarter's mean and reads as a
	// ±100% "trend". With no robust scale to measure against, only a
	// SUSTAINED move counts: the larger quarter level must reach the series'
	// own peak scale — a blip-dragged mean sits far below the blip itself,
	// while a real zero→nonzero shift holds the quarter at the new level.
	if f.TrendHit {
		med := median(vals)
		if madDenominator(mad(vals, med), med) < minMadScale {
			first, last, _ := quarterMeans(vals)
			peak := math.Max(math.Abs(f.Stats.Max), math.Abs(f.Stats.Min))
			if math.Max(math.Abs(first), math.Abs(last)) < 0.5*peak {
				f.TrendHit = false
			}
		}
	}

	f.YoY = compareWindows(f.Stats, prevTs, prevVals)
	if f.YoY != nil && len(f.Outliers)+len(f.Spikes) > 0 {
		cur := anomalousPoints(ts, vals, step)
		prev := anomalousPoints(prevTs, prevVals, step)
		f.Periodic = allRecurInPrev(cur, prev, shift, step)
	}

	// Suspicion score: how many detectors fired (dominant term) plus the
	// largest magnitudes (tie-breaker). Periodic recurrence without a level
	// shift means "same as yesterday" — downgrade out of the suspicious
	// bucket entirely; with a level shift (trend/yoy) it stays suspicious.
	hits := 0
	magSum := 0.0
	if len(f.Outliers) > 0 {
		hits++
		magSum += magTerm(maxAbsPct(f.Outliers))
	}
	if len(f.Spikes) > 0 {
		hits++
		magSum += magTerm(maxAbsPct(f.Spikes))
	}
	if f.TrendHit {
		hits++
		magSum += magTerm(math.Abs(f.TrendPct))
	}
	if f.YoY != nil && f.YoY.Hit {
		hits++
		magSum += magTerm(math.Abs(f.YoY.AvgPct))
	}
	f.Score = float64(hits)*1000 + magSum
	f.Suspicious = hits > 0
	if f.Periodic && !f.TrendHit && (f.YoY == nil || !f.YoY.Hit) {
		f.Suspicious = false
		f.Score *= 0.3
	}
	return f
}

func maxAbsPct(marks []pointMark) float64 {
	m := 0.0
	for _, p := range marks {
		if a := math.Abs(p.Pct); a > m {
			m = a
		}
	}
	return m
}

// downsampleForDisplay picks ~maxPoints evenly spaced indices and force-merges
// the marked anomalous points so the LLM always sees the evidence (a uniform
// stride alone can step right over a one-point spike). Returns parallel slices.
func downsampleForDisplay(ts []int64, vals []float64, marks []pointMark, maxPoints int) ([]int64, []float64) {
	n := len(ts)
	if maxPoints <= 0 || n <= maxPoints {
		return ts, vals
	}
	keep := make(map[int64]bool, len(marks))
	for _, m := range marks {
		keep[m.Ts] = true
	}
	stride := (n + maxPoints - 1) / maxPoints
	outTs := make([]int64, 0, maxPoints+len(marks))
	outVals := make([]float64, 0, maxPoints+len(marks))
	for i := 0; i < n; i++ {
		if i%stride == 0 || i == n-1 || keep[ts[i]] {
			outTs = append(outTs, ts[i])
			outVals = append(outVals, vals[i])
		}
	}
	return outTs, outVals
}
