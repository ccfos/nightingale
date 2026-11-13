package router

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/gin-gonic/gin"
)

type batchParallelDS struct {
	datasource.Datasource
	started chan string
	release <-chan struct{}
}

func (d batchParallelDS) QueryLog(ctx context.Context, q interface{}) ([]interface{}, int64, error) {
	select {
	case d.started <- fmt.Sprint(q):
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
	select {
	case <-d.release:
		return []interface{}{map[string]any{"message": "other"}}, 1, nil
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
}
func TestDorisLogBatchParallelBeforeFirstRow(t *testing.T) {
	for _, kind := range []string{"same_pool", "different_pools", "mixed", "probe"} {
		t.Run(kind, func(t *testing.T) {
			started := make(chan string, 6)
			release := make(chan struct{})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			state := &batchStreamDB{firstRows: started, releaseFirstRows: release}
			first := batchStreamClient(t, 889901, state)
			cached, _ := pool.PoolClient.Load(first.Addr + "::")
			cached.(*sql.DB).SetMaxOpenConns(100)
			queries := []Query{streamBatchQuery("A", "SELECT 1 LIMIT 2", 889901), streamBatchQuery("B", "SELECT 2 LIMIT 2", 889901), streamBatchQuery("C", "SELECT 3 LIMIT 2", 889901)}
			clients := []datasource.Datasource{first, first, first}
			if kind == "different_pools" {
				for i := 1; i < 3; i++ {
					clients[i] = batchStreamClient(t, 889901+int64(i), state)
				}
			}
			if kind == "mixed" {
				for _, i := range []int{0, 2} {
					queries[i].DsCate = "other"
					clients[i] = batchParallelDS{started: started, release: release}
				}
			}
			if kind == "probe" {
				for i := range queries {
					queries[i].Query = map[string]any{"sql": fmt.Sprintf("SELECT %d", i+1)}
				}
			}
			rec := httptest.NewRecorder()
			engine := gin.New()
			engine.Use(aop.Recovery())
			engine.POST("/", func(c *gin.Context) {
				StreamLogQueryBatch(c, queries, LogQueryBatchStreamOptions{Concurrent: true, Prepare: func(i int) (datasource.Datasource, error) { return clients[i], nil }})
			})
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				engine.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil).WithContext(ctx))
			}()
			count := 0
			for count < 3 {
				select {
				case <-started:
					count++
				case <-ctx.Done():
					close(release)
					<-finished
					t.Fatalf("only %d queries reached first row concurrently", count)
				}
			}
			close(release)
			<-finished
			var body struct {
				Err string `json:"err"`
				Dat struct {
					Total int64 `json:"total"`
					List  []struct {
						Ref string `json:"ref"`
					} `json:"list"`
				} `json:"dat"`
			}
			if e := json.Unmarshal(rec.Body.Bytes(), &body); e != nil {
				t.Fatal(e)
			}
			total := int64(6)
			if kind == "mixed" {
				total = 4
			}
			if body.Err != "" || body.Dat.Total != total || len(body.Dat.List) != 3 {
				t.Fatalf("body=%s", rec.Body.Bytes())
			}
			for i, ref := range []string{"A", "B", "C"} {
				if body.Dat.List[i].Ref != ref {
					t.Fatalf("order=%+v", body.Dat.List)
				}
			}
		})
	}
}
