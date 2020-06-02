package cache

type Point struct {
	Key       string  `msg:"key"`
	Timestamp int64   `msg:"timestamp"`
	Value     float64 `msg:"value"`
}
