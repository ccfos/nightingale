package tdigest

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

func TestBasics(t *testing.T) {
	s := newSummary(2)

	for _, n := range []float64{12, 13, 14, 15} {
		item := s.Find(n)

		if item.isValid() {
			t.Errorf("Found something for non existing key %.0f: %v", n, item)
		}
	}

	err := s.Add(1, 1)

	if err != nil {
		t.Errorf("Failed to add simple item")
	}

	if s.Add(math.NaN(), 1) == nil {
		t.Errorf("Adding math.NaN() shouldn't be allowed")
	}

	if s.Add(1, 0) == nil {
		t.Errorf("Adding count=0 shouldn't be allowed")
	}
}

func checkSorted(s *summary, t *testing.T) {
	if !sort.Float64sAreSorted(s.keys) {
		t.Fatalf("Keys are not sorted! %v", s.keys)
	}
}

func TestCore(t *testing.T) {

	testData := make(map[float64]uint32)

	const maxDataSize = 10000
	s := newSummary(maxDataSize)
	checkSorted(s, t)

	if s.Len() != 0 {
		t.Errorf("Initial size should be zero regardless of capacity. Got %d", s.Len())
	}

	for i := 0; i < maxDataSize; i++ {
		k := rand.Float64()
		v := rand.Uint32()

		err := s.Add(k, v)

		if err != nil {
			_, exists := testData[k]
			if !exists {
				t.Errorf("Failed to insert %.2f even though it doesn't exist yet", k)
			}
		}

		testData[k] = v
	}

	checkSorted(s, t)

	if s.Len() != len(testData) {
		t.Errorf("Got Len() == %d. Expected %d", s.Len(), len(testData))
	}

	for k, v := range testData {
		c := s.Find(k)
		if !c.isValid() || c.count != v {
			t.Errorf("Find(%.0f) returned %d, expected %d", k, c.count, v)
		}
	}
}

func TestGetAt(t *testing.T) {
	data := make(map[int]uint32)
	const maxDataSize = 1000

	s := newSummary(maxDataSize)

	c := s.At(0)

	if c.isValid() {
		t.Errorf("At() on an empty structure should give invalid data. Got %v", c)
	}

	for i := 0; i < maxDataSize; i++ {
		data[i] = rand.Uint32()
		s.Add(float64(i), data[i])
	}

	for i, v := range data {
		c := s.At(i)
		if !c.isValid() || c.count != v {
			t.Errorf("At(%d) = %d. Should've been %d", i, c.count, v)
		}
	}

	c = s.At(s.Len())

	if c.isValid() {
		t.Errorf("At() past the slice length should give invalid data")
	}

	c = s.At(-10)

	if c.isValid() {
		t.Errorf("At() with negative index should give invalid data")
	}
}

func TestIterate(t *testing.T) {

	s := newSummary(10)
	for _, i := range []uint32{1, 2, 3, 4, 5, 6} {
		s.Add(float64(i), i*10)
	}

	c := 0
	s.Iterate(func(i centroid) bool {
		c++
		return false
	})

	if c != 1 {
		t.Errorf("Iterate must exit early if the closure returns false")
	}

	var tot uint32
	s.Iterate(func(i centroid) bool {
		tot += i.count
		return true
	})

	if tot != 210 {
		t.Errorf("Iterate must walk through the whole data if it always returns true")
	}
}

func TestCeilingAndFloor(t *testing.T) {
	s := newSummary(100)

	ceil, floor := s.ceilingAndFloorItems(1)

	if ceil.isValid() || floor.isValid() {
		t.Errorf("Empty centroids must return invalid ceiling and floor items")
	}

	s.Add(0.4, 1)

	ceil, floor = s.ceilingAndFloorItems(0.3)

	if floor.isValid() || ceil.mean != 0.4 {
		t.Errorf("Expected to find a ceil and NOT find a floor. ceil=%v, floor=%v", ceil, floor)
	}

	ceil, floor = s.ceilingAndFloorItems(0.5)

	if ceil.isValid() || floor.mean != 0.4 {
		t.Errorf("Expected to find a floor and NOT find a ceiling. ceil=%v, floor=%v", ceil, floor)
	}

	s.Add(0.1, 2)

	ceil, floor = s.ceilingAndFloorItems(0.2)

	if ceil.mean != 0.4 || floor.mean != 0.1 {
		t.Errorf("Expected to find a ceiling and a floor. ceil=%v, floor=%v", ceil, floor)
	}

	s.Add(0.21, 3)

	ceil, floor = s.ceilingAndFloorItems(0.2)

	if ceil.mean != 0.21 || floor.mean != 0.1 {
		t.Errorf("Ceil should've shrunk. ceil=%v, floor=%v", ceil, floor)
	}

	s.Add(0.1999, 1)

	ceil, floor = s.ceilingAndFloorItems(0.2)

	if ceil.mean != 0.21 || floor.mean != 0.1999 {
		t.Errorf("Floor should've shrunk. ceil=%v, floor=%v", ceil, floor)
	}

	ceil, floor = s.ceilingAndFloorItems(10)

	if ceil.isValid() {
		t.Errorf("Expected an invalid ceil. Got %v", ceil)
	}

	ceil, floor = s.ceilingAndFloorItems(0.0001)

	if floor.isValid() {
		t.Errorf("Expected an invalid floor. Got %v", floor)
	}

	m := float64(0.42)
	s.Add(m, 1)
	ceil, floor = s.ceilingAndFloorItems(m)

	if ceil.mean != m || floor.mean != m {
		t.Errorf("ceiling and floor of an existing item should be the item itself")
	}
}

func TestAdjustLeftRight(t *testing.T) {

	keys := []float64{1, 2, 3, 4, 9, 5, 6, 7, 8}
	counts := []uint32{1, 2, 3, 4, 9, 5, 6, 7, 8}

	s := summary{keys: keys, counts: counts}

	s.adjustRight(4)

	if !sort.Float64sAreSorted(s.keys) || s.counts[4] != 5 {
		t.Errorf("adjustRight should have fixed the keys/counts state. %v %v", s.keys, s.counts)
	}

	keys = []float64{1, 2, 3, 4, 0, 5, 6, 7, 8}
	counts = []uint32{1, 2, 3, 4, 0, 5, 6, 7, 8}

	s = summary{keys: keys, counts: counts}
	s.adjustLeft(4)

	if !sort.Float64sAreSorted(s.keys) || s.counts[4] != 4 {
		t.Errorf("adjustLeft should have fixed the keys/counts state. %v %v", s.keys, s.counts)
	}
}
