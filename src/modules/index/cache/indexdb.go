package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/toolkits/compress"
	"github.com/didi/nightingale/src/toolkits/identity"
	"github.com/didi/nightingale/src/toolkits/report"
)

type CacheSection struct {
	CacheDuration   int    `yaml:"cacheDuration"`
	CleanInterval   int    `yaml:"cleanInterval"`
	PersistInterval int    `yaml:"persistInterval"`
	PersistDir      string `yaml:"persistDir"`
	RebuildWorker   int    `yaml:"rebuildWorker"`
	MaxQueryCount   int    `yaml:"maxQueryCount"`
	ReportEndpoint  bool   `yaml:"reportEndpoint"`
}

var IndexDB *EndpointIndexMap
var Config CacheSection
var NewEndpoints *list.SafeListLimited

var semaPermanence = semaphore.NewSemaphore(1)

func InitDB(cfg CacheSection) {
	Config = cfg

	IndexDB = &EndpointIndexMap{M: make(map[string]*MetricIndexMap, 0)}
	NewEndpoints = list.NewSafeListLimited(100000)

	Rebuild(Config.PersistDir, Config.RebuildWorker)

	go StartCleaner(Config.CleanInterval, Config.CacheDuration)
	go StartPersist(Config.PersistInterval)

	if Config.ReportEndpoint {
		go ReportEndpoint()
	}
}

func StartCleaner(interval int, cacheDuration int) {
	t1 := time.NewTicker(time.Duration(interval) * time.Second)
	for {
		<-t1.C

		start := time.Now()
		IndexDB.Clean(int64(cacheDuration))
		logger.Infof("clean took %.2f ms\n", float64(time.Since(start).Nanoseconds())*1e-6)
	}
}

func StartPersist(interval int) {
	t1 := time.NewTicker(time.Duration(interval) * time.Second)
	for {
		<-t1.C

		err := Persist("normal")
		if err != nil {
			logger.Error("Persist err:", err)
		}
		//logger.Infof("clean %+v, took %.2f ms\n", cleanRet, float64(time.Since(start).Nanoseconds())*1e-6)
	}
}

func Rebuild(persistenceDir string, concurrency int) {
	var dbDir string
	indexList := IndexList()
	if len(indexList) > 0 {
		err := getIndexFromRemote(indexList)
		if err == nil {
			dbDir = fmt.Sprintf("%s/%s", persistenceDir, "download")
		}
	}

	if dbDir == "" { //dbDir为空说明从远端下载索引失败，从本地读取
		logger.Debug("rebuild from local")
		dbDir = fmt.Sprintf("%s/%s", persistenceDir, "db")
	}

	err := RebuildFromDisk(dbDir, concurrency)
	if err != nil {
		logger.Error(err)
	}
}

func RebuildFromDisk(indexFileDir string, concurrency int) error {
	logger.Info("Try to rebuild index from disk")
	if !file.IsExist(indexFileDir) {
		return fmt.Errorf("index persistence dir %s not exists.", indexFileDir)
	}

	//遍历目录
	files, err := ioutil.ReadDir(indexFileDir)
	if err != nil {
		return err
	}
	logger.Infof("There're [%d] endpoints need rebuild", len(files))

	sema := semaphore.NewSemaphore(concurrency)
	for _, fileObj := range files {
		if fileObj.IsDir() {
			continue
		}
		endpoint := fileObj.Name()

		sema.Acquire()
		go func(endpoint string) {
			defer sema.Release()

			metricIndexMap, err := ReadIndexFromFile(indexFileDir, endpoint)
			if err != nil {
				logger.Errorf("read file error, [endpoint:%s][reason:%v]", endpoint, err)
				return
			}

			if !metricIndexMap.IsReported() {
				NewEndpoints.PushFront(endpoint) //没有标记上报过的endpoint，重新上报给monapi
			}

			IndexDB.Lock()
			IndexDB.M[endpoint] = metricIndexMap
			IndexDB.Unlock()
		}(endpoint)

	}
	logger.Infof("rebuild from disk done")
	return nil
}

func Persist(mode string) error {
	indexFileDir := Config.PersistDir
	if mode == "end" {
		semaPermanence.Acquire()
		defer semaPermanence.Release()

	} else if mode == "normal" || mode == "download" {
		if !semaPermanence.TryAcquire() {
			return fmt.Errorf("permanence operate is Already running...")
		}
		defer semaPermanence.Release()
	} else {
		return fmt.Errorf("wrong mode:%v", mode)
	}

	var tmpDir string
	if mode == "download" {
		tmpDir = fmt.Sprintf("%s/%s", indexFileDir, "download")
	} else {
		tmpDir = fmt.Sprintf("%s/%s", indexFileDir, "tmp")
	}
	if err := os.RemoveAll(tmpDir); err != nil {
		return err
	}
	//创建tmp目录
	if err := os.MkdirAll(tmpDir, 0777); err != nil {
		return err
	}

	//填充tmp目录
	endpoints := IndexDB.GetEndpoints()
	logger.Infof("save index data to disk[num:%d][mode:%s]\n", len(endpoints), mode)

	for i, endpoint := range endpoints {
		logger.Infof("sync [%s] to disk, [%d%%] complete\n", endpoint, int((float64(i)/float64(len(endpoints)))*100))

		err := WriteIndexToFile(tmpDir, endpoint)
		if err != nil {
			logger.Errorf("write %s index to file err:%v", endpoint, err)
		}
	}

	logger.Infof("sync to disk , [%d%%] complete\n", 100)

	if mode == "download" {
		compress.TarGz(fmt.Sprintf("%s/%s", indexFileDir, "db.tar.gz"), tmpDir)
	}

	//清空旧的db目录
	oleIndexDir := fmt.Sprintf("%s/%s", indexFileDir, "db")
	if err := os.RemoveAll(oleIndexDir); err != nil {
		return err
	}
	//将tmp目录改名为正式的文件名
	if err := os.Rename(tmpDir, oleIndexDir); err != nil {
		return err
	}

	return nil
}

func WriteIndexToFile(indexDir, endpoint string) error {
	metricIndexMap, exists := IndexDB.GetMetricIndexMap(endpoint)
	if !exists || metricIndexMap == nil {
		return fmt.Errorf("endpoint index not found")
	}

	metricIndexMap.Lock()
	body, err := json.Marshal(metricIndexMap)
	metricIndexMap.Unlock()
	if err != nil {
		return fmt.Errorf("marshal struct to json failed:%v", err)
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/%s", indexDir, endpoint), body, 0666)
	return err
}

func ReadIndexFromFile(indexDir, endpoint string) (*MetricIndexMap, error) {
	metricIndexMap := new(MetricIndexMap)

	body, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", indexDir, endpoint))
	if err != nil {
		return metricIndexMap, err
	}

	err = json.Unmarshal(body, metricIndexMap)
	return metricIndexMap, err
}

func IndexList() []*model.Instance {
	var instances []*model.Instance
	activeIndexs, _ := report.GetAlive("index", "monapi")
	for _, instance := range activeIndexs {
		if instance.Identity != identity.Identity {
			instances = append(instances, instance)
		}
	}
	return instances
}

func getIndexFromRemote(instances []*model.Instance) error {
	filepath := fmt.Sprintf("db.tar.gz")
	var err error
	// Get the data

	perm := rand.Perm(len(instances))
	for i := range perm {
		url := fmt.Sprintf("http://%s:%s/api/index/idxfile", instances[perm[i]].Identity, instances[perm[i]].HTTPPort)
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// Create the file
		out, err := os.Create(filepath)
		if err != nil {
			return err
		}
		defer out.Close()
		// Write the body to file
		_, err = io.Copy(out, resp.Body)
	}

	compress.UnTarGz(filepath, ".")
	if err != nil {
		return err
	}
	//清空db目录
	if err = os.Remove(filepath); err != nil {
		return err
	}

	return err
}
