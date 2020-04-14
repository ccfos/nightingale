package judge

import (
	"strings"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/judge/cache"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/logger"
)

var nodataJob *semaphore.Semaphore

func NodataJudge(concurrency int) {
	if concurrency < 1 {
		concurrency = 1000
	}
	nodataJob = semaphore.NewSemaphore(concurrency)

	t1 := time.NewTicker(time.Duration(9000) * time.Millisecond)
	nodataJudge()
	for {
		<-t1.C
		nodataJudge()
	}
}

func nodataJudge() {
	stras := cache.NodataStra.GetAll()
	for _, stra := range stras {
		//nodata处理
		now := time.Now().Unix()
		respData, err := GetData(stra, stra.Exprs[0], nil, now, false)
		if err != nil {
			logger.Errorf("stra:%v get query data err:%v", stra, err)
			//获取数据报错，直接出发nodata
			for _, endpoint := range stra.Endpoints {
				if endpoint == "" {
					continue
				}
				judgeItem := &dataobj.JudgeItem{
					Endpoint: endpoint,
					Metric:   stra.Exprs[0].Metric,
					Tags:     "",
					DsType:   "GAUGE",
				}

				nodataJob.Acquire()
				go AsyncJudge(nodataJob, stra, stra.Exprs, []*dataobj.RRDData{}, judgeItem, now, []dataobj.History{}, "", "", "", []bool{})
			}
			return
		}

		for _, data := range respData {
			var metric, tag string
			arr := strings.Split(data.Counter, "/")
			if len(arr) == 2 {
				metric = arr[0]
				tag = arr[1]
			} else {
				metric = data.Counter
			}

			if data.Endpoint == "" {
				continue
			}
			judgeItem := &dataobj.JudgeItem{
				Endpoint: data.Endpoint,
				Metric:   metric,
				Tags:     tag,
				DsType:   data.DsType,
				Step:     data.Step,
			}

			nodataJob.Acquire()
			go AsyncJudge(nodataJob, stra, stra.Exprs, data.Values, judgeItem, now, []dataobj.History{}, "", "", "", []bool{})
		}
	}
}

func AsyncJudge(sema *semaphore.Semaphore, stra *model.Stra, exps []model.Exp, historyData []*dataobj.RRDData, firstItem *dataobj.JudgeItem, now int64, history []dataobj.History, info string, value string, extra string, status []bool) {
	defer sema.Release()
	Judge(stra, exps, historyData, firstItem, now, history, info, value, extra, status)
}
