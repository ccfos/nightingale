// Package tdigest provides a highly accurate mergeable data-structure
// for quantile estimation.
package tdigest

import (
	"fmt"
	"math"
	"math/rand"
)

// TDigest is a quantile approximation data structure.
// Typical T-Digest use cases involve accumulating metrics on several
// distinct nodes of a cluster and then merging them together to get
// a system-wide quantile overview. Things such as: sensory data from
// IoT devices, quantiles over enormous document datasets (think
// ElasticSearch), performance metrics for distributed systems, etc.
type TDigest struct {
	summary     *summary
	compression float64
	count       uint32
}

// New creates a new digest.
// The compression parameter rules the threshold in which samples are
// merged together - the more often distinct samples are merged the more
// precision is lost. Compression should be tuned according to your data
// distribution, but a value of 100 is often good enough. A higher
// compression value means holding more centroids in memory (thus: better
// precision), which means a bigger serialization payload and higher
// memory footprint.
// Compression must be a value greater of equal to 1, will panic
// otherwise.
func New(compression float64) *TDigest {
	if compression < 1 {
		panic("Compression must be >= 1.0")
	}
	return &TDigest{
		compression: compression,
		summary:     newSummary(estimateCapacity(compression)),
		count:       0,
	}
}

// Quantile returns the desired percentile estimation.
// Values of p must be between 0 and 1 (inclusive), will panic otherwise.
func (t *TDigest) Quantile(q float64) float64 {
	if q < 0 || q > 1 {
		panic("q must be between 0 and 1 (inclusive)")
	}

	if t.summary.Len() == 0 {
		return math.NaN()
	} else if t.summary.Len() == 1 {
		return t.summary.Min().mean
	}

	q *= float64(t.count)
	var total float64
	i := 0

	found := false
	var result float64

	t.summary.Iterate(func(item centroid) bool {
		k := float64(item.count)

		if q < total+k {
			if i == 0 || i+1 == t.summary.Len() {
				result = item.mean
				found = true
				return false
			}
			succ, pred := t.summary.successorAndPredecessorItems(item.mean)
			delta := (succ.mean - pred.mean) / 2
			result = item.mean + ((q-total)/k-0.5)*delta
			found = true
			return false
		}

		i++
		total += k
		return true
	})

	if found {
		return result
	}
	return t.summary.Max().mean
}

// Add registers a new sample in the digest.
// It's the main entry point for the digest and very likely the only
// method to be used for collecting samples. The count parameter is for
// when you are registering a sample that occurred multiple times - the
// most common value for this is 1.
func (t *TDigest) Add(value float64, count uint32) error {

	if count == 0 {
		return fmt.Errorf("Illegal datapoint <value: %.4f, count: %d>", value, count)
	}

	if t.summary.Len() == 0 {
		t.summary.Add(value, count)
		t.count = count
		return nil
	}

	// Avoid allocation for our slice by using a local array here.
	ar := [2]centroid{}
	candidates := ar[:]
	candidates[0], candidates[1] = t.findNearestCentroids(value)
	if !candidates[1].isValid() {
		candidates = candidates[:1]
	}
	for len(candidates) > 0 && count > 0 {
		j := 0
		if len(candidates) > 1 {
			j = rand.Intn(len(candidates))
		}
		chosen := candidates[j]

		quantile := t.computeCentroidQuantile(&chosen)

		if float64(chosen.count+count) > t.threshold(quantile) {
			candidates = append(candidates[:j], candidates[j+1:]...)
			continue
		}

		t.summary.updateAt(chosen.index, value, uint32(count))
		t.count += count
		count = 0
	}

	if count > 0 {
		t.summary.Add(value, count)
		t.count += count
	}

	if float64(t.summary.Len()) > 20*t.compression {
		t.Compress()
	}

	return nil
}

// Compress tries to reduce the number of individual centroids stored
// in the digest.
// Compression trades off accuracy for performance and happens
// automatically after a certain amount of distinct samples have been
// stored.
func (t *TDigest) Compress() {
	if t.summary.Len() <= 1 {
		return
	}

	oldTree := t.summary
	t.summary = newSummary(estimateCapacity(t.compression))
	t.count = 0

	nodes := oldTree.Data()
	shuffle(nodes)

	for _, item := range nodes {
		t.Add(item.mean, item.count)
	}
}

// Merge joins a given digest into itself.
// Merging is useful when you have multiple TDigest instances running
// in separate threads and you want to compute quantiles over all the
// samples. This is particularly important on a scatter-gather/map-reduce
// scenario.
func (t *TDigest) Merge(other *TDigest) {
	if other.summary.Len() == 0 {
		return
	}

	nodes := other.summary.Data()
	shuffle(nodes)

	for _, item := range nodes {
		t.Add(item.mean, item.count)
	}
}

// Len returns the number of centroids in the TDigest.
func (t *TDigest) Len() int { return t.summary.Len() }

// ForEachCentroid calls the specified function for each centroid.
// Iteration stops when the supplied function returns false, or when all
// centroids have been iterated.
func (t *TDigest) ForEachCentroid(f func(mean float64, count uint32) bool) {
	s := t.summary
	for i := 0; i < s.Len(); i++ {
		if !f(s.keys[i], s.counts[i]) {
			break
		}
	}
}

func shuffle(data []centroid) {
	for i := len(data) - 1; i > 1; i-- {
		other := rand.Intn(i + 1)
		tmp := data[other]
		data[other] = data[i]
		data[i] = tmp
	}
}

func estimateCapacity(compression float64) uint {
	return uint(compression) * 10
}

func (t *TDigest) threshold(q float64) float64 {
	return (4 * float64(t.count) * q * (1 - q)) / t.compression
}

func (t *TDigest) computeCentroidQuantile(c *centroid) float64 {
	cumSum := t.summary.sumUntilMean(c.mean)
	return (float64(c.count)/2.0 + float64(cumSum)) / float64(t.count)
}

func (t *TDigest) findNearestCentroids(mean float64) (centroid, centroid) {
	ceil, floor := t.summary.ceilingAndFloorItems(mean)

	if !ceil.isValid() && !floor.isValid() {
		panic("findNearestCentroids called on an empty tree")
	}

	if !ceil.isValid() {
		return floor, invalidCentroid
	}

	if !floor.isValid() {
		return ceil, invalidCentroid
	}

	if math.Abs(floor.mean-mean) < math.Abs(ceil.mean-mean) {
		return floor, invalidCentroid
	} else if math.Abs(floor.mean-mean) == math.Abs(ceil.mean-mean) && floor.mean != ceil.mean {
		return floor, ceil
	} else {
		return ceil, invalidCentroid
	}
}
