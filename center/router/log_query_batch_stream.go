package router

import (
	"context"
	"fmt"
	"math"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/datasource/doris"
	dskittypes "github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/pkg/logx"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

// LogQueryBatchStreamOptions preserves each entry point's preparation,
// query-error messages and execution policy while sharing the streaming engine.
// Prepare runs for every query before any query starts or response is written.
type LogQueryBatchStreamOptions struct {
	Prepare    func(int) (datasource.Datasource, error)
	QueryError func(int, error) error
	Concurrent bool
}

// StreamLogQueryBatch writes the existing grouped JSON response, reading Doris
// rows incrementally. Other datasources keep their existing QueryLog contract.
func StreamLogQueryBatch(c *gin.Context, queries []Query, options LogQueryBatchStreamOptions) {
	response := NewLogQueryResponse(c)
	clients := make([]datasource.Datasource, len(queries))
	contexts := make([]context.Context, len(queries))
	operator := ginUser(c)
	for i, q := range queries {
		plug, err := options.Prepare(i)
		if err != nil {
			response.Fail(err)
			return
		}
		if q.DsCate == "doris" {
			if _, ok := plug.(*doris.Doris); !ok {
				response.Fail(fmt.Errorf("cluster not doris"))
				return
			}
		}
		clients[i] = plug
		contexts[i] = withCallContext(c.Request.Context(), q.Did, operator)
	}
	formatError := func(i int, err error) error {
		if err == nil {
			return nil
		}
		logx.Warningf(contexts[i], "query data error: %v query:%v", err, queries[i])
		if options.QueryError != nil {
			return options.QueryError(i, err)
		}
		return err
	}
	var total int64
	if options.Concurrent {
		var err error
		total, err = streamLogBatchConcurrently(c.Request.Context(), response, queries, clients, contexts, formatError)
		if err != nil {
			response.Fail(err)
			return
		}
	} else {
		for i, q := range queries {
			n, err := writeLogBatchQuery(response, q, func(visit func(map[string]any) error) ([]any, int64, error) {
				data, count, err := readLogBatchQuery(contexts[i], q, clients[i], visit)
				return data, count, formatError(i, err)
			})
			if err != nil {
				response.Fail(err)
				return
			}
			total += n
		}
	}
	if err := response.Finish(total, nil); err != nil {
		response.Fail(err)
	}
}

func readLogBatchQuery(ctx context.Context, q Query, client datasource.Datasource, visit func(map[string]any) error) ([]any, int64, error) {
	if q.DsCate == "doris" {
		count, err := client.(*doris.Doris).QueryLogStream(ctx, q.Query, math.MaxInt64, visit)
		return nil, count, err
	}
	return client.QueryLog(ctx, q.Query)
}

func writeLogBatchQuery(response *LogQueryResponse, q Query, read func(func(map[string]any) error) ([]any, int64, error)) (int64, error) {
	if q.DsCate == "doris" {
		if err := response.beginBatchQuery(q); err != nil {
			return 0, err
		}
	}
	data, total, err := read(response.Row)
	if err != nil {
		return 0, err
	}
	if q.DsCate == "doris" {
		err = response.endBatchQuery()
	} else {
		err = response.Row(map[string]any{"ref": q.Ref, "ds_id": q.Did, "ds_cate": q.DsCate, "data": data})
	}
	return total, err
}

func streamLogBatchConcurrently(ctx context.Context, response *LogQueryResponse, queries []Query, clients []datasource.Datasource, contexts []context.Context, formatError func(int, error) error) (int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	group, queryCtx := errgroup.WithContext(ctx)
	type result struct {
		rows  chan map[string]any
		ready chan struct{}
		data  []any
		total int64
		err   error
	}
	results := make([]result, len(queries))
	for i, q := range queries {
		results[i].rows = make(chan map[string]any)
		results[i].ready = make(chan struct{})
		current := &results[i]
		var previous <-chan struct{}
		if i > 0 {
			previous = results[i-1].ready
		}
		callContext, _ := dskittypes.CallContextFromCtx(contexts[i])
		queryContext := dskittypes.WithCallContext(queryCtx, callContext)
		group.Go(func() error {
			defer close(current.rows)
			ready := false
			signal := func() {
				if !ready {
					close(current.ready)
					ready = true
				}
			}
			defer signal()
			// Acquire cursors in output order so a later query cannot occupy the
			// only pool connection while an earlier query waits to be written.
			if previous != nil {
				select {
				case <-previous:
				case <-queryCtx.Done():
					current.err = queryCtx.Err()
					return current.err
				}
			}
			data, total, err := readLogBatchQuery(queryContext, q, clients[i], func(row map[string]any) error {
				signal()
				select {
				case current.rows <- row:
					return nil
				case <-queryCtx.Done():
					return queryCtx.Err()
				}
			})
			current.data, current.total, current.err = data, total, err
			return formatError(i, err)
		})
	}
	var total int64
	for i, q := range queries {
		current := &results[i]
		n, err := writeLogBatchQuery(response, q, func(visit func(map[string]any) error) ([]any, int64, error) {
			for row := range current.rows {
				if err := visit(row); err != nil {
					cancel()
					_ = group.Wait()
					return nil, 0, err
				}
			}
			if current.err != nil {
				return nil, 0, group.Wait()
			}
			return current.data, current.total, nil
		})
		if err != nil {
			cancel()
			_ = group.Wait()
			return 0, err
		}
		total += n
	}
	return total, group.Wait()
}
