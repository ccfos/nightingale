package writer

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/prompb"
)

func TestQueueExpiryAndConcurrentRecreation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const id = "lifecycle-test"
		const producers = 64
		ws := &WritersType{queues: make(map[string]*IdentQueue)}
		ws.pushgw.WriterOpt.QueueMaxSize = producers
		// Keep entries queued so the test can account for every producer's write.
		ws.pushgw.WriterOpt.QueuePopSize = 0

		var previous *IdentQueue
		for round := 0; round < 2; round++ {
			start := make(chan struct{})
			var wg sync.WaitGroup
			for range producers {
				wg.Go(func() {
					<-start
					if err := ws.PushSample(id, prompb.TimeSeries{}); err != nil {
						t.Error(err)
					}
				})
			}
			close(start)
			wg.Wait()
			synctest.Wait()

			queue := ws.queues[id]
			if queue == nil || queue == previous {
				t.Fatal("expected a newly registered queue")
			}
			if n := queue.list.Len(); n != producers {
				t.Fatalf("registered queue got %d of %d entries", n, producers)
			}
			time.Sleep(15 * time.Second)
			synctest.Wait()
			if n := testutil.ToFloat64(pstat.GaugeSampleQueueSize.WithLabelValues(id)); n != producers {
				t.Fatalf("reported length = %v, want %d", n, producers)
			}
			ws.cleanExpiredQueues(queue.ts.Load() + 3601)
			time.Sleep(400 * time.Millisecond) // Let the idle consumer wake and observe closure.
			synctest.Wait()
			if ws.queues[id] != nil {
				t.Fatal("expired queue is still registered")
			}
			previous = queue
		}
	})
}

func TestQueueReporterStopsOnClose(t *testing.T) {
	ws := &WritersType{}
	q := &IdentQueue{closeCh: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		ws.ReportQueueStats("closed", q)
		close(done)
	}()
	close(q.closeCh)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reporter still running after queue closed")
	}
}
