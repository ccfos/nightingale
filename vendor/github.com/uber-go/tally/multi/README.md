# A buffered multi reporter

Combine reporters to emit to many backends.

Multiple `tally.StatsReporter` as a single reporter:
```go
reporter := NewMultiReporter(statsdReporter, ...)
```

Multiple `tally.CachedStatsReporter` as a single reporter:
```go
reporter := NewMultiCachedReporter(m3Reporter, promReporter, ...)
```
