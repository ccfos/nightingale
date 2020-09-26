package worker

import (
	"fmt"
	"log"
	"testing"
	"time"
)

func TestCreatejobAndDeletejob(t *testing.T) {
	config := &ConfigInfo{
		Id:       1,
		FilePath: "a.log.${%Y-%m-%d-%H}",
	}
	cache := make(chan string, 100)

	go func() {
		time.Sleep(2 * time.Second)
		deleteJob(config)
	}()
	if err := createJob(config, cache); err == nil {
		for line := range cache {
			fmt.Println(line)
		}
	} else {
		log.Printf("create job failed : %v", err)
	}
}
