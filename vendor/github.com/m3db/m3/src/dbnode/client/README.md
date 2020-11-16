# `client` Implementation details

This file lists details and design choices about the implementation of the `client` package.


## FetchTagged/FetchTaggedIDs
First point to note, `FetchTagged`/`FetchTaggedIDs` share a majority of code, so the following applies to both.
The document points out where they differ as applicable.

First a quick glossary of the types used in this flow:
- `session`: the implementation backing the `client.Session` interface, it's topology aware and does
the fan-out/coordination of the request/response lifecycle.
- `op`: generic interface used by the `session` as a unit of work. It has a `completionFn` specified based
on the backing implementation. It is executed by the `hostQueue`.
- `completionFn`: the callback specified on each `op` to return a response to the caller.
- `hostQueue`: work queue maintained per host by the `session`. It does the execution of a rpc request
asynchronously, and executing any callbacks specified on response/timeout.
- `fetchState`: struct corresponding to a single attempt of a `FetchTagged` rpc request.
- `fetchTaggedResultsAccumulator`: struct used by a `fetchState` for accumulating responses and converting to
the appropriate response types eventually.
- `fetchTaggedShardConsistencyResult`: struct used by the `fetchTaggedResultsAccumulator` for response
consistency calculations.
- `fetchTaggedOp`: implementation of `op` for the `FetchTagged` method calls.

Easiest way to understand this is to follow the control flow for the execution of a FetchTagged method.

Sequence of steps:
1. A user of the API calls `session.FetchTagged(...)`
2. `session` validates the request, and converts to the rpc type; terminating early if this fails.
3. Once the request is validated, the `session` retrieves a `fetchState`, and `fetchTaggedOp`
from their respective pools. We create a single `fetchState` and `fetchTaggedOp` per attempt.
4. The `fetchTaggedOp` is enqueued into each known host's `hostQueue`.
5. The `session` then calls `fetchState.Wait()`, which is backed by a condition variable, while
waiting for responses to come back. Note: the calling go-routine will keep waiting until sufficient
responses have been received to fullfil the consistency requirement of the request, or if we receive
responses from all hosts and are still unable to meet the constraint.
6. In the background, each `hostQueue` is executing the appropriate rpc and calling the `completionFn`
with success/error/timeout.
7. For each execution of the `completionFn`, the `fetchState` passes the response to the
`fetchTaggedResultsAccumulator` which does the consistency calculation and updates its internal state.
The `fetchTaggedResultsAccumulator` returns whether it's hit a terminating condition to to the `fetchState`,
which acts upon it and if it has, calls `Signal()` to indicate to the original go-routine waiting on the
condition variable that a response is available.

### Nuances about lifecycle
- The `fetchTaggedOp` retains a reference to the `fetchState`, so the `fetchState` has to remain alive
until all hosts have returned a response/timed-out.
- The `fetchState` created by the session in steps during the execution of a `FetchTagged()` method can
remain alive even after the response. This is because we can return success even if we haven't received
responses from all hosts depending on the consistency requirement. As a result, the final caller to
`fetchState.decRef()` actually cleans it up and returns it to the pool. This will be a hostQueue in case
of early success, or the go-routine calling the the `FetchTagged()` in case of error.
