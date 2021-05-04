# T-Digest

A map-reduce and parallel streaming friendly data-structure for accurate
quantile approximation.

This package provides a very crude implementation of Ted Dunning's t-digest
data structure in Go.

[![Build Status](https://travis-ci.org/caio/go-tdigest.svg?branch=master)](https://travis-ci.org/caio/go-tdigest)
[![GoDoc](https://godoc.org/github.com/caio/go-tdigest?status.svg)](http://godoc.org/github.com/caio/go-tdigest)
[![Coverage](http://gocover.io/_badge/github.com/caio/go-tdigest)](http://gocover.io/github.com/caio/go-tdigest)
[![Go Report Card](https://goreportcard.com/badge/github.com/caio/go-tdigest)](https://goreportcard.com/report/github.com/caio/go-tdigest)

## Installation

    go get github.com/caio/go-tdigest

## Usage

    package main

    import (
            "fmt"
            "math/rand"

            "github.com/caio/go-tdigest"
    )

    func main() {
            var t = tdigest.New(100)

            for i := 0; i < 10000; i++ {
                    t.Add(rand.Float64(), 1)
            }

            fmt.Printf("p(.5) = %.6f\n", t.Quantile(0.5))
    }

## Disclaimer

I've written this solely with the purpose of understanding how the
data-structure works, it hasn't been throughly verified nor battle tested
in a production environment.

## References

This is a very simple port of the [reference][1] implementation with some
ideas borrowed from the [python version][2]. If you wanna get a quick grasp of
how it works and why it's useful, [this video and companion article is pretty
helpful][3].

[1]: https://github.com/tdunning/t-digest
[2]: https://github.com/CamDavidsonPilon/tdigest
[3]: https://www.mapr.com/blog/better-anomaly-detection-t-digest-whiteboard-walkthrough

