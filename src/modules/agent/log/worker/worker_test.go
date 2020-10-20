package worker

import (
	"fmt"
	"testing"
	"time"
)

func TestWorkerStart(t *testing.T) {
	c := make(chan string, 10)
	go func() {
		for i := 0; i < 1000; i++ {
			for j := 0; j < 10; j++ {
				c <- fmt.Sprintf("test--%d--%d", i, j)
			}
			fmt.Println()
			time.Sleep(time.Second * 1)
		}
	}()

	wg := NewWorkerGroup("test", c)
	wg.Start()
	time.Sleep(10 * time.Second)
	wg.Stop()
	time.Sleep(1 * time.Second)
}
