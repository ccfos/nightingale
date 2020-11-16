package stackadler32

var prime uint32 = 65521

// Digest computes an adler32 hash and will very likely
// be allocated on the stack when used locally and not casted
// to an interface.
type Digest struct {
	initialized bool
	s1          uint32
	s2          uint32
}

// NewDigest returns an adler32 digest struct.
func NewDigest() Digest {
	return Digest{
		initialized: true,
		s1:          1 & 0xffff,
		s2:          (1 >> 16) & 0xffff,
	}
}

// Update returns a new derived adler32 digest struct.
func (d Digest) Update(buf []byte) Digest {
	r := d
	if !r.initialized {
		r = NewDigest()
	}
	for n := 0; n < len(buf); n++ {
		r.s1 = (r.s1 + uint32(buf[n])) % prime
		r.s2 = (r.s2 + r.s1) % prime
	}
	return r
}

// Sum32 returns the currently computed adler32 hash.
func (d Digest) Sum32() uint32 {
	if !d.initialized {
		return NewDigest().Sum32()
	}
	return ((d.s2 << 16) | d.s1)
}

// Checksum returns an adler32 checksum of the buffer specified.
func Checksum(buf []byte) uint32 {
	return NewDigest().Update(buf).Sum32()
}
