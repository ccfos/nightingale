package tdigest

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const smallEncoding int32 = 2

var endianess = binary.BigEndian

// AsBytes serializes the digest into a byte array so it can be
// saved to disk or sent over the wire.
func (t TDigest) AsBytes() ([]byte, error) {
	buffer := new(bytes.Buffer)

	err := binary.Write(buffer, endianess, smallEncoding)

	if err != nil {
		return nil, err
	}

	err = binary.Write(buffer, endianess, t.compression)

	if err != nil {
		return nil, err
	}

	err = binary.Write(buffer, endianess, int32(t.summary.Len()))

	if err != nil {
		return nil, err
	}

	var x float64
	t.summary.Iterate(func(item centroid) bool {
		delta := item.mean - x
		x = item.mean
		err = binary.Write(buffer, endianess, float32(delta))

		return err == nil
	})
	if err != nil {
		return nil, err
	}

	t.summary.Iterate(func(item centroid) bool {
		err = encodeUint(buffer, item.count)
		return err == nil
	})
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// FromBytes reads a byte buffer with a serialized digest (from AsBytes)
// and deserializes it.
func FromBytes(buf *bytes.Reader) (*TDigest, error) {
	var encoding int32
	err := binary.Read(buf, endianess, &encoding)
	if err != nil {
		return nil, err
	}

	if encoding != smallEncoding {
		return nil, fmt.Errorf("Unsupported encoding version: %d", encoding)
	}

	var compression float64
	err = binary.Read(buf, endianess, &compression)
	if err != nil {
		return nil, err
	}

	t := New(compression)

	var numCentroids int32
	err = binary.Read(buf, endianess, &numCentroids)
	if err != nil {
		return nil, err
	}

	if numCentroids < 0 || numCentroids > 1<<22 {
		return nil, errors.New("bad number of centroids in serialization")
	}

	means := make([]float64, numCentroids)
	var delta float32
	var x float64
	for i := 0; i < int(numCentroids); i++ {
		err = binary.Read(buf, endianess, &delta)
		if err != nil {
			return nil, err
		}
		x += float64(delta)
		means[i] = x
	}

	for i := 0; i < int(numCentroids); i++ {
		decUint, err := decodeUint(buf)
		if err != nil {
			return nil, err
		}

		t.Add(means[i], decUint)
	}

	return t, nil
}

func encodeUint(buf *bytes.Buffer, n uint32) error {
	var b [binary.MaxVarintLen32]byte

	l := binary.PutUvarint(b[:], uint64(n))

	buf.Write(b[:l])

	return nil
}

func decodeUint(buf *bytes.Reader) (uint32, error) {
	v, err := binary.ReadUvarint(buf)
	if v > 0xffffffff {
		return 0, errors.New("Something wrong, this number looks too big")
	}
	return uint32(v), err
}
