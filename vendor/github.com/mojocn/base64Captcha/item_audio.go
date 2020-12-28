// Copyright 2017 Eric Zhou. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base64Captcha

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
)

//ItemAudio captcha-audio-engine return type.
type ItemAudio struct {
	answer      string
	body        *bytes.Buffer
	digitSounds [][]byte
	//rng         siprng
}

// newAudio returns a new audio captcha with the given digits, where each digit
// must be in range 0-9. Digits are pronounced in the given language. If there
// are no sounds for the given language, English is used.
// Possible values for lang are "en", "ja", "ru", "zh".
func newAudio(id string, digits []byte, lang string) *ItemAudio {
	a := new(ItemAudio)

	if sounds, ok := digitSounds[lang]; ok {
		a.digitSounds = sounds
	} else {
		a.digitSounds = digitSounds["en"]
	}
	numsnd := make([][]byte, len(digits))
	for i, n := range digits {
		snd := a.randomizedDigitSound(n)
		setSoundLevel(snd, 1.5)
		numsnd[i] = snd
	}
	// Random intervals between digits (including beginning).
	intervals := make([]int, len(digits)+1)
	intdur := 0
	for i := range intervals {
		dur := randIntRange(sampleRate, sampleRate*2) // 1 to 2 seconds
		intdur += dur
		intervals[i] = dur
	}
	// Generate background sound.
	bg := a.makeBackgroundSound(a.longestDigitSndLen()*len(digits) + intdur)
	// Create buffer and write audio to it.
	sil := makeSilence(sampleRate / 5)
	bufcap := 3*len(beepSound) + 2*len(sil) + len(bg) + len(endingBeepSound)
	a.body = bytes.NewBuffer(make([]byte, 0, bufcap))
	// Write prelude, three beeps.
	a.body.Write(beepSound)
	a.body.Write(sil)
	a.body.Write(beepSound)
	a.body.Write(sil)
	a.body.Write(beepSound)
	// Write digits.
	pos := intervals[0]
	for i, v := range numsnd {
		mixSound(bg[pos:], v)
		pos += len(v) + intervals[i+1]
	}
	a.body.Write(bg)
	// Write ending (one beep).
	a.body.Write(endingBeepSound)
	return a
}

// encodedLen returns the length of WAV-encoded audio captcha.
func (a *ItemAudio) encodedLen() int {
	return len(waveHeader) + 4 + a.body.Len()
}

func (a *ItemAudio) makeBackgroundSound(length int) []byte {
	b := a.makeWhiteNoise(length, 4)
	for i := 0; i < length/(sampleRate/10); i++ {
		snd := reversedSound(a.digitSounds[rand.Intn(10)])
		//snd = changeSpeed(snd, a.rng.Float(0.8, 1.2))
		place := rand.Intn(len(b) - len(snd))
		setSoundLevel(snd, randFloat64Range(0.04, 0.08))
		mixSound(b[place:], snd)
	}
	return b
}

func (a *ItemAudio) randomizedDigitSound(n byte) []byte {
	s := a.randomSpeed(a.digitSounds[n])
	setSoundLevel(s, randFloat64Range(0.85, 1.2))
	return s
}

func (a *ItemAudio) longestDigitSndLen() int {
	n := 0
	for _, v := range a.digitSounds {
		if n < len(v) {
			n = len(v)
		}
	}
	return n
}

func (a *ItemAudio) randomSpeed(b []byte) []byte {
	pitch := randFloat64Range(0.95, 1.1)
	return changeSpeed(b, pitch)
}

func (a *ItemAudio) makeWhiteNoise(length int, level uint8) []byte {
	noise := randBytes(length)
	adj := 128 - level/2
	for i, v := range noise {
		v %= level
		v += adj
		noise[i] = v
	}
	return noise
}

// WriteTo writes captcha audio in WAVE format into the given io.Writer, and
// returns the number of bytes written and an error if any.
func (a *ItemAudio) WriteTo(w io.Writer) (n int64, err error) {
	// Calculate padded length of PCM chunk data.
	bodyLen := uint32(a.body.Len())
	paddedBodyLen := bodyLen
	if bodyLen%2 != 0 {
		paddedBodyLen++
	}
	totalLen := uint32(len(waveHeader)) - 4 + paddedBodyLen
	// Header.
	header := make([]byte, len(waveHeader)+4) // includes 4 bytes for chunk size
	copy(header, waveHeader)
	// Put the length of whole RIFF chunk.
	binary.LittleEndian.PutUint32(header[4:], totalLen)
	// Put the length of WAVE chunk.
	binary.LittleEndian.PutUint32(header[len(waveHeader):], bodyLen)
	// Write header.
	nn, err := w.Write(header)
	n = int64(nn)
	if err != nil {
		return
	}
	// Write data.
	n, err = a.body.WriteTo(w)
	n += int64(nn)
	if err != nil {
		return
	}
	// Pad byte if chunk length is odd.
	// (As header has even length, we can check if n is odd, not chunk).
	if bodyLen != paddedBodyLen {
		w.Write([]byte{0})
		n++
	}
	return
}

// EncodeB64string encodes a sound to base64 string
func (a *ItemAudio) EncodeB64string() string {
	var buf bytes.Buffer
	if _, err := a.WriteTo(&buf); err != nil {
		panic(err)
	}
	return fmt.Sprintf("data:%s;base64,%s", MimeTypeAudio, base64.StdEncoding.EncodeToString(buf.Bytes()))

}
