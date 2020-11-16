// +build !roaringstats

package roaring

// statsCount does nothing, because you aren't building with
// the "roaringstats" build tag.
func statsHit(string) {
}
