# Unreleased

## Doris queries

- Add optional row-consumer methods to the Doris client and datasource without changing the public datasource interface. The existing collected-result methods reuse the same row reader.
- Read Doris rows incrementally in `/api/n9e/log-query-batch`, preserving successful response fields, result ordering, authorization and the existing behavior of other datasources. Consumers apply backpressure; cancellation closes unread results without draining them.

## Upgrade notes

- When a query fails after the HTTP response has started, the batch endpoint completes the existing JSON envelope with a nonempty `err` and `dat.total` set to 0. `dat.list` can contain results already transmitted. Callers must treat nonempty `err` as failure and discard these results. Transport failures can still interrupt the response.
- Streaming reduces result buffering; it does not impose a process or container memory limit. SQL validation and the existing row-count probe remain in effect.
