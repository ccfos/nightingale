package worker

import (
	"sync"
	"time"

	"github.com/didi/nightingale/src/modules/collector/log/reader"
	"github.com/didi/nightingale/src/modules/collector/log/strategy"
	"github.com/toolkits/pkg/logger"
)

type ConfigInfo struct {
	Id       int64
	FilePath string
}

type Job struct {
	r *reader.Reader
	w *WorkerGroup
}

var ManagerJob map[string]*Job //管理job,文件路径为key
var ManagerJobLock *sync.RWMutex
var ManagerConfig map[int64]*ConfigInfo

func init() {
	ManagerJob = make(map[string]*Job)
	ManagerJobLock = new(sync.RWMutex)
	ManagerConfig = make(map[int64]*ConfigInfo)
}

func UpdateConfigsLoop() {
	for {
		strategy.Update()
		strategyMap := strategy.GetAll() //最新策略

		ManagerJobLock.Lock()
		//metric.MetricJobNum(len(ManagerJob))
		for id, st := range strategyMap {
			cfg := &ConfigInfo{
				Id:       id,
				FilePath: st.FilePath,
			}
			cache := make(chan string, WorkerConfig.QueueSize)
			if err := createJob(cfg, cache); err != nil {
				logger.Errorf("create job fail [id:%d][filePath:%s][err:%v]", cfg.Id, cfg.FilePath, err)
			}
		}

		for id := range ManagerConfig {
			if _, ok := strategyMap[id]; !ok { //如果策略中不存在，说明用户已删除
				cfg := &ConfigInfo{
					Id:       id,
					FilePath: ManagerConfig[id].FilePath,
				}
				deleteJob(cfg)
			}
		}
		ManagerJobLock.Unlock()

		//更新counter
		GlobalCount.UpdateByStrategy(strategyMap)
		time.Sleep(time.Second * 10)
	}
}

func GetLatestTmsAndDelay(filepath string) (int64, int64, bool) {
	ManagerJobLock.RLock()
	job, ok := ManagerJob[filepath]
	ManagerJobLock.RUnlock()

	if !ok {
		return 0, 0, false
	}
	latest, delay := job.w.GetLatestTmsAndDelay()
	return latest, delay, true
}

//添加任务到管理map( managerjob managerconfig) 启动reader和worker
func createJob(config *ConfigInfo, cache chan string) error {
	if _, ok := ManagerJob[config.FilePath]; ok {
		if _, ok := ManagerConfig[config.Id]; !ok {
			ManagerConfig[config.Id] = config
		}
		//依赖策略的周期更新, 触发文件乱序时间戳的重置
		ManagerJob[config.FilePath].w.ResetMaxDelay()
		return nil
	}

	ManagerConfig[config.Id] = config
	//启动reader
	r, err := reader.NewReader(config.FilePath, cache)
	if err != nil {
		return err
	}
	//metric.MetricReadAddReaderNum(config.FilePath)
	//启动worker
	w := NewWorkerGroup(config.FilePath, cache)
	ManagerJob[config.FilePath] = &Job{
		r: r,
		w: w,
	}
	w.Start()
	//启动reader
	go r.Start()

	logger.Infof("Create job success [filePath:%s][sid:%d]", config.FilePath, config.Id)
	return nil
}

//先stop worker reader再从管理map中删除
func deleteJob(config *ConfigInfo) {
	//删除jobs
	tag := 0
	for _, cg := range ManagerConfig {
		if config.FilePath == cg.FilePath {
			tag++
		}
	}
	if tag <= 1 {
		//metric.MetricReadDelReaderNum(config.FilePath)
		if job, ok := ManagerJob[config.FilePath]; ok {
			job.w.Stop() //先stop worker
			job.r.Stop()
			delete(ManagerJob, config.FilePath)
		}
	}
	logger.Infof("Stop reader & worker success [filePath:%s][sid:%d]", config.FilePath, config.Id)

	//删除config
	if _, ok := ManagerConfig[config.Id]; ok {
		delete(ManagerConfig, config.Id)
	}
}

func Zeroize() {

	t1 := time.NewTicker(time.Duration(10) * time.Second)
	for {
		<-t1.C
		stras := strategy.GetListAll()
		for _, stra := range stras {
			if stra.Func == "cnt" && len(stra.Tags) < 1 {
				point := &AnalysPoint{
					StrategyID: stra.ID,
					Value:      -1,
					Tms:        time.Now().Unix(),
					Tags:       map[string]string{},
				}
				if err := PushToCount(point); err != nil {
					logger.Errorf("push to counter error: %v", err)
				}
			}
		}
	}

}
