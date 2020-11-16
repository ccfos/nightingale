# series

Series related documentation.

## Series flush lifecycle

Warm/cold writes end up in versioned buckets based on write type (`ColdWrite` or `WarmWrite`). When flushes occur, we fetch in-mem data from all write type specific buckets to persist.

For warm flushes, we write all warm written buckets to disk and mark the state of the block as `WarmRetrievable`.

For cold flushes, we merge this data w/ data that's already on disk in `fs/merger.go` and write to disk. Once finished, we then update the `ColdVersionRetrievable` to the cold version we just wrote to disk.

Data is only evicted from mem during a `Tick()`. This evicts either cold buckets up until flush state `ColdVersionRetrievable` or warm buckets that are marked as `WarmRetrievable` (or warm blocks that we have already warm flushed to disk).

## Snapshotting/Bootstrap

Snapshots work by merging all buckets for a series buffer regardless of write type into streams and persisting to disk. Snapshots are in the commitlog bootstrapper and snapshotted series data are loaded into `BufferBucket.loadedBlocks`. Attempts to call `series.LoadBlock()` for `WarmWrite` blocks will return an error if it already exists on disk.

Series snapshots persist writes in both warm & cold buckets. During a flush, we persist snapshot files w/ a commit log ID. This ID is later used during the async cleanup process to deleted rotated commit logs.

## Repair

Shard repairs load data as cold writes into series buffer buckets.
