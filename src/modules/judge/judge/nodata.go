package judge

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/judge/backend/redi"
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
	for {
		if time.Now().Unix()%10 == 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	t1 := time.NewTicker(time.Duration(10) * time.Second)
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
		if len(stra.Endpoints) == 0 && len(stra.Nids) == 0 {
			logger.Debugf("stra:%+v endpoints or nids is null", stra)
			continue
		}

		now := time.Now().Unix()
		respData, err := GetData(stra, stra.Exprs[0], nil, now, false)
		if err != nil {
			logger.Errorf("stra:%+v get query data err:%v", stra, err)
			continue
		}

		for _, data := range respData {
			var metric, tag string
			// 兼容格式disk.bytes.free/mount=/data/docker/overlay2/xxx/merged
			arr := strings.SplitN(data.Counter, "/", 2)
			if len(arr) == 2 {
				metric = arr[0]
				tag = arr[1]
			} else {
				metric = data.Counter
			}

			if data.Endpoint == "" && data.Nid == "" {
				continue
			}

			judgeItem := &dataobj.JudgeItem{
				Nid:      data.Nid,
				Endpoint: data.Endpoint,
				Metric:   metric,
				Tags:     tag,
				TagsMap:  dataobj.DictedTagstring(tag),
				DsType:   data.DsType,
				Step:     data.Step,
			}

			nodataJob.Acquire()
			go AsyncJudge(nodataJob, stra, stra.Exprs, dataobj.RRDData2HistoryData(data.Values), judgeItem, now)
		}
	}
}

func AsyncJudge(sema *semaphore.Semaphore, stra *models.Stra, exps []models.Exp, historyData []*dataobj.HistoryData, firstItem *dataobj.JudgeItem, now int64) {
	defer sema.Release()

	historyArr := []dataobj.History{}
	statusArr := []bool{}
	eventInfo := ""
	value := ""

	for _, expr := range exps {
		respData, err := GetData(stra, expr, firstItem, now, true)
		if err != nil {
			logger.Errorf("stra:%+v get query data err:%v", stra, err)
			return
		}

		if len(respData) != 1 {
			logger.Errorf("stra:%+v get query data respData:%v err", stra, respData)
			return
		}

		history, info, lastValue, status := Judge(stra, expr, dataobj.RRDData2HistoryData(respData[0].Values), firstItem, now)

		statusArr = append(statusArr, status)
		if value == "" {
			value = fmt.Sprintf("%s: %s", expr.Metric, lastValue)
		} else {
			value += fmt.Sprintf("; %s: %s", expr.Metric, lastValue)
		}

		historyArr = append(historyArr, history)
		eventInfo += info
	}

	bs, err := json.Marshal(historyArr)
	if err != nil {
		logger.Errorf("Marshal history:%+v err:%v", historyArr, err)
	}

	event := &dataobj.Event{
		ID:        fmt.Sprintf("s_%d_%s", stra.Id, firstItem.PrimaryKey()),
		Etime:     now,
		Endpoint:  firstItem.Endpoint,
		CurNid:    firstItem.Nid,
		Info:      eventInfo,
		Detail:    string(bs),
		Value:     value,
		Partition: redi.Config.Prefix + "/event/p" + strconv.Itoa(stra.Priority),
		Sid:       stra.Id,
		Hashid:    getHashId(stra.Id, firstItem),
	}

	sendEventIfNeed(historyData, statusArr, event, stra)
}
