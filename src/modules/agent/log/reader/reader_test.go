package reader

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"
)

//当新文件出现时，是否自动读取
func TestCheck(t *testing.T) {
	util(true)
}

//测试文件打开关闭
func TestStartAndStop(t *testing.T) {
	util(false)
}

func util(isnext bool) {
	stream := make(chan string, 100)
	rj, err := NewReader("/test/aby.${%Y-%m-%d-%H}", stream)
	if err != nil {
		return
	}
	go rj.Start()
	go func() {
		time.Sleep(2 * time.Second) //2秒后创建文件
		now := time.Now()
		if isnext {
			now = now.Add(time.Hour)
		}
		suffix := now.Format("2006-01-02-15")
		filepath := fmt.Sprintf("/test/aby.%s", suffix)

		{
			f, err := os.Create(filepath)
			if err != nil {
				log.Fatal(err)
			}
			time.Sleep(time.Millisecond * 250) //因为tail 的巡检周期是250ms
			defer f.Close()

			fmt.Fprint(f, "this is a test\n")

		}
		time.Sleep(250 * time.Millisecond) //延迟关闭
		rj.Stop()

	}()

	for line := range stream {
		fmt.Println(line)
	}
}
