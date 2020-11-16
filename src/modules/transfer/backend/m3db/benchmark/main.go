package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/backend/m3db"
	"github.com/toolkits/pkg/concurrent/semaphore"
	"gopkg.in/yaml.v2"
)

func getConf() m3db.M3dbSection {
	c := m3db.M3dbSection{}
	b, err := ioutil.ReadFile("benchmark.yml")
	if err != nil {
		log.Fatalf("readfile benchmark.yml err %s", err)
	}
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	return c
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	cfg := getConf()
	cli, err := m3db.NewClient(cfg)
	if err != nil {
		log.Fatalf("newclient err %s", err)
	}
	defer cli.Close()

	if len(os.Args) != 2 {
		fmt.Printf("usage: %s <QPS>\n", os.Args[0])
		os.Exit(1)
	}
	qps, err := strconv.Atoi(os.Args[1])
	if err != nil || qps <= 0 {
		fmt.Printf("usage: %s <QPS>\n", os.Args[0])
		os.Exit(1)
	}

	t0 := time.Duration(float64(100000*time.Second) / float64(qps))

	log.Printf("qps %d (10w per %.3fs)", qps, float64(t0)/float64(time.Second))

	sema := semaphore.NewSemaphore(100)
	t1 := time.NewTicker(t0)
	log.Println("begin...")

	for {
		<-t1.C
		for i := 0; i < 255; i++ {
			for j := 0; j < 4; j++ {
				endpoint := "192.168." + strconv.Itoa(i) + "." + strconv.Itoa(j)
				for k := 0; k < 1; k++ { //1000*100=10W
					metric := "metric." + strconv.Itoa(k)
					sema.Acquire()
					go func(endpoint, metric string) {
						defer sema.Release()
						items := getTransferItems(endpoint, metric)
						//log.Println(items[0])
						start := time.Now().UnixNano()
						err := cli.Push(items)
						if err != nil {
							fmt.Println("err:", err)
						} else {
							//fmt.Println("resp:", resp)
						}
						log.Println((time.Now().UnixNano() - start) / 1000000)
					}(endpoint, metric)
				}
			}
		}
		log.Println("push..")
	}
}

func getTransferItems(endpoint, metric string) []*dataobj.MetricValue {
	ret := []*dataobj.MetricValue{}
	now := time.Now().Unix()
	ts := now - now%10 // 对齐时间戳
	r1 := rand.Intn(100)

	l := rand.Intn(6) * 100
	if l == 0 {
		l = 100
	}

	for i := 0; i < 100; i++ {
		ret = append(ret, &dataobj.MetricValue{
			Endpoint: endpoint,
			Metric:   metric,
			TagsMap: map[string]string{
				"errno": fmt.Sprintf("tsdb.%d", i),
			},
			Value:       float64(r1),
			Timestamp:   ts,
			CounterType: "GAUGE",
			Step:        10,
		})
	}
	return ret
}
