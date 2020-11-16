// Copyright (c) 2015 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tos

// ToS represents a const value DF, CS3 etc
// Assured Forwarding (x=class, y=drop precedence) (RFC2597)
// Class Selector (RFC 2474)
// IP Precedence (Linux Socket Compat RFC 791
type ToS uint8

// Assured Forwarding (x=class, y=drop precedence) (RFC2597)
// Class Selector (RFC 2474)

const (
	// CS3 Class Selector 3
	CS3 ToS = 0x18
	// CS4 Class Selector 4
	CS4 ToS = 0x20
	// CS5 Class Selector 5
	CS5 ToS = 0x28
	// CS6 Class Selector 6
	CS6 ToS = 0x30
	// CS7 Class Selector 7
	CS7 ToS = 0x38
	// AF11 Assured Forward 11
	AF11 ToS = 0x0a
	// AF12 Assured Forward 11
	AF12 ToS = 0x0c
	// AF13 Assured Forward 12
	AF13 ToS = 0x0e
	// AF21 Assured Forward 13
	AF21 ToS = 0x12
	// AF22 Assured Forward 21
	AF22 ToS = 0x14
	// AF23 Assured Forward 22
	AF23 ToS = 0x16
	// AF31 Assured Forward 23
	AF31 ToS = 0x1a
	// AF32 Assured Forward 31
	AF32 ToS = 0x1c
	// AF33 Assured Forward 32
	AF33 ToS = 0x1e
	// AF41 Assured Forward 33
	AF41 ToS = 0x22
	// AF42 Assured Forward 41
	AF42 ToS = 0x24
	// AF43 Assured Forward 42
	AF43 ToS = 0x26
	// EF Expedited Forwarding (RFC 3246)
	EF ToS = 0x2e
	// Lowdelay 10
	Lowdelay ToS = 0x10
	// Throughput 8
	Throughput ToS = 0x08
	// Reliability  4
	Reliability ToS = 0x04
	// Lowcost 2
	Lowcost ToS = 0x02
)
