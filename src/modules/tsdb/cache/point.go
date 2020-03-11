package cache

type Point struct {
	Key       interface{} `msg:"key"`
	Timestamp int64       `msg:"timestamp"`
	Value     float64     `msg:"value"`
}
