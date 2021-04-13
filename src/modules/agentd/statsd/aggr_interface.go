package statsd

// interface aggregator
type aggregator interface {
	new(aggregatorNames []string) (aggregator, error)
	collect(values []string, metric string, argLines string) error
	dump(points []*Point, timestamp int64, tags map[string]string, metric string, argLines string) ([]*Point, error)
	summarize(nsmetric, argLines string, newAggrs map[string]aggregator)
	merge(toMerge aggregator) (aggregator, error)
	toMap() (map[string]interface{}, error)
	fromMap(map[string]interface{}) (aggregator, error)
}
