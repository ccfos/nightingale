package base64Captcha

import (
	"math"
)

const sampleRate = 8000 // Hz

var endingBeepSound []byte

func init() {
	endingBeepSound = changeSpeed(beepSound, 1.4)
}

// mixSound mixes src into dst. Dst must have length equal to or greater than
// src length.
func mixSound(dst, src []byte) {
	for i, v := range src {
		av := int(v)
		bv := int(dst[i])
		if av < 128 && bv < 128 {
			dst[i] = byte(av * bv / 128)
		} else {
			dst[i] = byte(2*(av+bv) - av*bv/128 - 256)
		}
	}
}

func setSoundLevel(a []byte, level float64) {
	for i, v := range a {
		av := float64(v)
		switch {
		case av > 128:
			if av = (av-128)*level + 128; av < 128 {
				av = 128
			}
		case av < 128:
			if av = 128 - (128-av)*level; av > 128 {
				av = 128
			}
		default:
			continue
		}
		a[i] = byte(av)
	}
}

// changeSpeed returns new PCM bytes from the bytes with the speed and pitch
// changed to the given value that must be in range [0, x].
func changeSpeed(a []byte, speed float64) []byte {
	b := make([]byte, int(math.Floor(float64(len(a))*speed)))
	var p float64
	for _, v := range a {
		for i := int(p); i < int(p+speed); i++ {
			b[i] = v
		}
		p += speed
	}
	return b
}

func makeSilence(length int) []byte {
	b := make([]byte, length)
	for i := range b {
		b[i] = 128
	}
	return b
}

func reversedSound(a []byte) []byte {
	n := len(a)
	b := make([]byte, n)
	for i, v := range a {
		b[n-1-i] = v
	}
	return b
}
