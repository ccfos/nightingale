package worker

import (
	"fmt"
	"math"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/log/strategy"
	"github.com/didi/nightingale/src/modules/collector/stra"

	"github.com/toolkits/pkg/logger"
)

//从worker往计算部分推的Point
type AnalysPoint struct {
	StrategyID int64
	Value      float64
	Tms        int64
	Tags       map[string]string
}

//统计的实体
type PointCounter struct {
	sync.RWMutex
	Count int64
	Sum   float64
	Max   float64
	Min   float64
}

// 单策略下，单step的统计对象
// 以Sorted的tagkv的字符串来做索引
type PointsCounter struct {
	sync.RWMutex
	TagstringMap map[string]*PointCounter
}

// 单策略下的对象, 以step为索引, 索引每一个Step的统计
// 单step统计, 推送完则删
type StrategyCounter struct {
	sync.RWMutex
	Strategy  *stra.Strategy           //Strategy结构体扔这里，以备不时之需
	TmsPoints map[int64]*PointsCounter //按照时间戳分类的分别的counter
}

// 全局counter对象, 以key为索引，索引每个策略的统计
// key : Strategy ID
type GlobalCounter struct {
	sync.RWMutex
	StrategyCounts map[int64]*StrategyCounter
}

var GlobalCount *GlobalCounter

func init() {
	GlobalCount = new(GlobalCounter)
	GlobalCount.StrategyCounts = make(map[int64]*StrategyCounter)
}

// 提供给Worker用来Push计算后的信息
// 需保证线程安全
func PushToCount(Point *AnalysPoint) error {
	stCount, err := GlobalCount.GetStrategyCountByID(Point.StrategyID)

	// 更新strategyCounts
	if err != nil {
		strategy, err := strategy.GetByID(Point.StrategyID)
		if err != nil {
			logger.Errorf("GetByID ERROR when count:[%v]", err)
			return err
		}

		GlobalCount.AddStrategyCount(strategy)

		stCount, err = GlobalCount.GetStrategyCountByID(Point.StrategyID)
		// 还拿不到，就出错返回吧
		if err != nil {
			logger.Errorf("Get strategyCount Failed after addition: %v", err)
			return err
		}
	}

	// 拿到stCount，更新StepCounts
	stepTms := AlignStepTms(stCount.Strategy.Interval, Point.Tms)
	tmsCount, err := stCount.GetByTms(stepTms)
	if err != nil {
		err := stCount.AddTms(stepTms)
		if err != nil {
			logger.Errorf("Add tms to strategy error: %v", err)
			return err
		}

		tmsCount, err = stCount.GetByTms(stepTms)
		// 还拿不到，就出错返回吧
		if err != nil {
			logger.Errorf("Get tmsCount Failed By Twice Add: %v", err)
			return err
		}
	}

	//拿到tmsCount, 更新TagstringMap
	tagstring := dataobj.SortedTags(Point.Tags)
	return tmsCount.Update(tagstring, Point.Value)
}

//时间戳向前对齐
func AlignStepTms(step, tms int64) int64 {
	if step <= 0 {
		return tms
	}
	newTms := tms - (tms % step)
	return newTms
}

func (this *PointsCounter) GetBytagstring(tagstring string) (*PointCounter, error) {
	this.RLock()
	point, ok := this.TagstringMap[tagstring]
	this.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tagstring [%s] not exists!", tagstring)
	}
	return point, nil
}

func (this *PointCounter) UpdateCnt() {
	atomic.AddInt64(&this.Count, 1)
}

func (this *PointCounter) UpdateSum(value float64) {
	addFloat64(&this.Sum, value)
}

func (this *PointCounter) UpdateMaxMin(value float64) {
	// 这里要用到结构体的小锁
	// sum和cnt可以不用锁，但是最大最小没办法做到原子操作
	// 只能引入锁
	this.RLock()
	if math.IsNaN(this.Max) || value > this.Max {
		this.RUnlock()
		this.Lock()
		if math.IsNaN(this.Max) || value > this.Max {
			this.Max = value
		}
		this.Unlock()
	} else {
		this.RUnlock()
	}

	this.RLock()
	if math.IsNaN(this.Min) || value < this.Min {
		this.RUnlock()
		this.Lock()
		if math.IsNaN(this.Min) || value < this.Min {
			this.Min = value
		}
		this.Unlock()
	} else {
		this.RUnlock()
	}
}

func (this *PointsCounter) Update(tagstring string, value float64) error {
	pointCount, err := this.GetBytagstring(tagstring)
	if err != nil {
		this.Lock()
		tmp := new(PointCounter)
		tmp.Count = 0
		tmp.Sum = 0

		if value == -1 {
			tmp.Sum = math.NaN() //补零逻辑，不处理Sum
		}
		tmp.Max = math.NaN()
		tmp.Min = math.NaN()
		this.TagstringMap[tagstring] = tmp
		this.Unlock()

		pointCount, err = this.GetBytagstring(tagstring)
		// 如果还是拿不到，就出错返回吧
		if err != nil {
			return fmt.Errorf("when update, cannot get pointCount after add [tagstring:%s]", tagstring)
		}
	}

	pointCount.Lock()

	if value != -1 { //value=-1,是补零逻辑，不做特殊处理
		pointCount.Sum = pointCount.Sum + value
		if math.IsNaN(pointCount.Max) || value > pointCount.Max {
			pointCount.Max = value
		}
		if math.IsNaN(pointCount.Min) || value < pointCount.Min {
			pointCount.Min = value
		}

		pointCount.Count = pointCount.Count + 1
	}

	pointCount.Unlock()

	return nil
}

func addFloat64(val *float64, delta float64) (new float64) {
	for {
		old := *val
		new = old + delta
		if atomic.CompareAndSwapUint64(
			(*uint64)(unsafe.Pointer(val)),
			math.Float64bits(old),
			math.Float64bits(new),
		) {
			break
		}
	}
	return
}

func (this *StrategyCounter) GetTmsList() []int64 {
	tmsList := []int64{}
	this.RLock()
	for tms := range this.TmsPoints {
		tmsList = append(tmsList, tms)
	}
	this.RUnlock()
	return tmsList
}

func (this *StrategyCounter) DeleteTms(tms int64) {
	this.Lock()
	delete(this.TmsPoints, tms)
	this.Unlock()
}

func (this *StrategyCounter) GetByTms(tms int64) (*PointsCounter, error) {
	this.RLock()
	psCount, ok := this.TmsPoints[tms]
	if !ok {
		this.RUnlock()
		return nil, fmt.Errorf("no this tms:%v", tms)
	}
	this.RUnlock()
	return psCount, nil
}

func (this *StrategyCounter) AddTms(tms int64) error {
	this.Lock()
	_, ok := this.TmsPoints[tms]
	if !ok {
		tmp := new(PointsCounter)
		tmp.TagstringMap = make(map[string]*PointCounter, 0)
		this.TmsPoints[tms] = tmp
	}
	this.Unlock()
	return nil
}

// 只做更新和删除，添加 由数据驱动
func (this *GlobalCounter) UpdateByStrategy(globalStras map[int64]*stra.Strategy) {
	var delCount, upCount int
	// 先以count的ID为准，更新count
	// 若ID没有了, 那就删掉
	for _, id := range this.GetIDs() {
		this.RLock()
		sCount, ok := this.StrategyCounts[id]
		this.RUnlock()

		if !ok || sCount.Strategy == nil {
			//证明此策略无效，或已被删除
			//删一下
			delCount = delCount + 1
			this.deleteByID(id)
			continue
		}

		newStrategy := globalStras[id]

		// 一个是sCount.Strategy, 一个是newStrategy
		// 开始比较
		if !countEqual(newStrategy, sCount.Strategy) {
			//需要清空缓存
			upCount = upCount + 1
			logger.Infof("strategy [%d] changed, clean data", id)
			this.cleanStrategyData(id)
			sCount.Strategy = newStrategy
		} else {
			this.upStrategy(newStrategy)
		}
	}
	logger.Infof("Update global count done, [del:%d][update:%d]", delCount, upCount)
}

func (this *GlobalCounter) AddStrategyCount(st *stra.Strategy) {
	this.Lock()
	if _, ok := this.StrategyCounts[st.ID]; !ok {
		tmp := new(StrategyCounter)
		tmp.Strategy = st
		tmp.TmsPoints = make(map[int64]*PointsCounter, 0)
		this.StrategyCounts[st.ID] = tmp
	}
	this.Unlock()
}

func (this *GlobalCounter) upStrategy(st *stra.Strategy) {
	this.Lock()
	if _, ok := this.StrategyCounts[st.ID]; ok {
		this.StrategyCounts[st.ID].Strategy = st
	}
	this.Unlock()
}

func (this *GlobalCounter) GetStrategyCountByID(id int64) (*StrategyCounter, error) {
	this.RLock()
	stCount, ok := this.StrategyCounts[id]
	if !ok {
		this.RUnlock()
		return nil, fmt.Errorf("No this ID")
	}
	this.RUnlock()
	return stCount, nil
}

func (this *GlobalCounter) GetIDs() []int64 {
	this.RLock()
	rList := make([]int64, 0)
	for k := range this.StrategyCounts {
		rList = append(rList, k)
	}
	this.RUnlock()
	return rList
}

func (this *GlobalCounter) deleteByID(id int64) {
	this.Lock()
	delete(this.StrategyCounts, id)
	this.Unlock()
}

func (this *GlobalCounter) cleanStrategyData(id int64) {
	this.RLock()
	sCount, ok := this.StrategyCounts[id]
	this.RUnlock()
	if !ok || sCount == nil {
		return
	}
	sCount.TmsPoints = make(map[int64]*PointsCounter, 0)
	return
}

// countEqual意味着不会对统计的结构产生影响
func countEqual(A *stra.Strategy, B *stra.Strategy) bool {
	if A == nil || B == nil {
		return false
	}
	if A.Pattern == B.Pattern && A.Interval == B.Interval && A.Func == B.Func && reflect.DeepEqual(A.Tags, B.Tags) {
		return true
	}
	return false

}
