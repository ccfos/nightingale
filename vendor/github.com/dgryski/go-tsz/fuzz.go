// +build gofuzz

package tsz

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/dgryski/go-tsz/testdata"
)

func Fuzz(data []byte) int {

	fuzzUnpack(data)

	if len(data) < 9 {
		return 0
	}

	t0 := uint32(1456236677)

	v := float64(10000)

	var vals []testdata.Point
	s := New(t0)
	t := t0
	for len(data) >= 10 {
		tdelta := uint32(binary.LittleEndian.Uint16(data))
		if t == t0 {
			tdelta &= (1 << 14) - 1
		}
		t += tdelta
		data = data[2:]
		v += float64(int16(binary.LittleEndian.Uint16(data))) + float64(binary.LittleEndian.Uint16(data[2:]))/float64(math.MaxUint16)
		data = data[8:]
		vals = append(vals, testdata.Point{V: v, T: t})
		s.Push(t, v)
	}

	it := s.Iter()

	var i int
	for it.Next() {
		gt, gv := it.Values()
		if gt != vals[i].T || (gv != vals[i].V || math.IsNaN(gv) && math.IsNaN(vals[i].V)) {
			panic(fmt.Sprintf("failure: gt=%v vals[i].T=%v gv=%v vals[i].V=%v", gt, vals[i].T, gv, vals[i].V))
		}
		i++
	}

	if i != len(vals) {
		panic("extra data")
	}

	return 1
}

func fuzzUnpack(data []byte) {

	it, err := NewIterator(data)
	if err != nil {
		return
	}

	for it.Next() {
		_, _ = it.Values()
	}
}
