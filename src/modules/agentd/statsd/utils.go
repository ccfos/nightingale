package statsd

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/spaolacci/murmur3"
)

type Func struct{}

var (
	BadRpcMetricError           = fmt.Errorf("bad rpc metric")
	BadSummarizeAggregatorError = fmt.Errorf("bad summarize aggregator")
	BadDeserializeError         = fmt.Errorf("bad deserialize")
	BadAggregatorNameError      = fmt.Errorf("bad aggregator name")

	cache *lru.Cache
)

func init() {
	cache, _ = lru.New(MaxLRUCacheSize)
}

type ArgCacheUnit struct {
	Aggrs   []string
	Tags    map[string]string
	ArgLine string
	Error   error
}

func NewArgCacheUnitWithError(err error) *ArgCacheUnit {
	return &ArgCacheUnit{
		Aggrs:   []string{},
		Tags:    make(map[string]string),
		ArgLine: "",
		Error:   err,
	}
}

func NewArgCacheUnit(argline string, aggrs []string,
	tags map[string]string) *ArgCacheUnit {
	return &ArgCacheUnit{
		Aggrs:   aggrs,
		Tags:    tags,
		ArgLine: argline,
		Error:   nil,
	}
}

// tags+aggr lines
func (f Func) FormatArgLines(argLines string, metricLines string) (string, []string, error) {
	// BUG: hash碰撞下可能出现问题, 暂时不处理
	key := murmur3.Sum32([]byte(argLines))
	value, found := cache.Get(key)
	if found {
		unit, ok := value.(*ArgCacheUnit)
		if ok {
			return unit.ArgLine, unit.Aggrs, unit.Error
		}
	}

	tags, agg, err := f.TranslateArgLines(argLines, true)
	if err != nil {
		cache.Add(key, NewArgCacheUnitWithError(err))
		return "", []string{}, fmt.Errorf("translate to tags error, [lines: %s][error: %s]", argLines, err.Error())
	}

	// check
	if err := f.checkTags(tags); err != nil {
		cache.Add(key, NewArgCacheUnitWithError(err))
		return "", []string{}, err
	}
	aggrs, err := f.formatAggr(agg)
	if err != nil {
		cache.Add(key, NewArgCacheUnitWithError(err))
		return "", []string{}, err
	}

	if len(tags) == 0 {
		cache.Add(key, NewArgCacheUnit(argLines, aggrs, tags))
		return argLines, aggrs, nil
	}

	traceExist := false
	if traceid, found := tags[TagTraceId]; found {
		traceExist = true
		delete(tags, TagTraceId)
		ignore := traceHandler.collectAndIgnore(metricLines, traceid)
		if ignore {
			return "", []string{}, fmt.Errorf("ignore")
		}
	}

	newLines := []string{}

	var keys []string
	for k, _ := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := tags[k]
		if v == "<all>" { // <all>是关键字, 需要去重
			v = "all"
			tags[k] = v // 缓存的tags 需要更新,保持一致
		}
		newLines = append(newLines, fmt.Sprintf("%s=%s", k, v))
	}

	newLines = append(newLines, agg)
	newArgLines := strings.Join(newLines, "\n")
	// 包含了traceid, 没有必要缓存, 基本不会命中
	if !traceExist {
		cache.Add(key, NewArgCacheUnit(newArgLines, aggrs, tags))
		// argLine重新排序后发生了变化(tag map有关), 新的argLine也要缓存
		if argLines != newArgLines {
			newKey := murmur3.Sum32([]byte(newArgLines))
			cache.Add(newKey, NewArgCacheUnit(newArgLines, aggrs, tags))
		}
	}

	return newArgLines, aggrs, nil
}

func (f Func) GetAggrsFromArgLines(argLines string) ([]string, error) {
	key := murmur3.Sum32([]byte(argLines))
	value, found := cache.Get(key)
	if found {
		unit, ok := value.(*ArgCacheUnit)
		if ok {
			return unit.Aggrs, unit.Error
		}
	}

	lines := strings.Split(argLines, "\n")
	lineSize := len(lines)
	if lineSize == 0 {
		return nil, fmt.Errorf("empty aggr")
	}

	return strings.Split(lines[lineSize-1], ","), nil
}

func (f Func) TranslateArgLines(argLines string, aggrNeed ...bool) (map[string]string, string, error) {
	// 只需要提取tags参数, 尝试从缓存中获取
	if len(aggrNeed) == 0 {
		key := murmur3.Sum32([]byte(argLines))
		value, found := cache.Get(key)
		if found {
			unit, ok := value.(*ArgCacheUnit)
			if ok {
				return unit.Tags, "", unit.Error
			}
		}
	}

	// 缓存中不存在, 执行解析 or 不允许从缓存中查询
	tags := make(map[string]string)
	lines := strings.Split(argLines, "\n")
	lineSize := len(lines)
	if lineSize == 0 {
		return tags, "", fmt.Errorf("empty aggr")
	}

	agg := lines[lineSize-1]
	if lineSize == 1 {
		return tags, agg, nil
	}

	for _, line := range lines[:lineSize-1] {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			tags[parts[0]] = parts[1]
		} else {
			return nil, "", fmt.Errorf("bad tag [%s]", line)
		}
	}

	return tags, agg, nil
}

func (f Func) checkTags(tags map[string]string) error {
	tcnt := len(tags)
	if tcnt > MaxTagsCntConst {
		return fmt.Errorf("too many tags %v", tags)
	}

	return nil
}

func (f Func) TrimRpcCallee(callee string) string {
	callee = strings.Replace(callee, "://", "|", -1)
	return strings.Replace(callee, ":", "|", -1)
}

// metric line: $ns/$raw-metric
func (f Func) FormatMetricLine(metricLine string, aggrs []string) (string, error) {
	ret, err := f.TranslateMetricLine(metricLine)
	if err != nil {
		return "", err
	}

	if len(ret) != 2 {
		return "", fmt.Errorf("bad metric line, missing ns or metric")
	}

	// ns
	ns := ret[0]
	if !strings.HasPrefix(ns, NsPrefixConst) {
		ns = NsPrefixConst + ns
	}
	if !strings.HasSuffix(ns, NsSuffixConst) {
		ns = ns + NsSuffixConst
	}

	// metric
	metric := ret[1]
	if len(aggrs) > 0 &&
		(aggrs[0] == Const_CommonAggregator_Rpc || aggrs[0] == Const_CommonAggregator_RpcE) {
		// metric: rpc统计类型 必须以rpc开头
		if !strings.HasPrefix(metric, "rpc") {
			metric = "rpc_" + metric
		}
	}

	return fmt.Sprintf("%s/%s", ns, metric), nil
}

func (f Func) TranslateMetricLine(metricLine string) ([]string, error) {
	return strings.SplitN(metricLine, "/", 2), nil
}

// aggr line
func (f Func) formatAggr(aggr string) ([]string, error) {
	aggrNames, err := f.translateAggregator(aggr)
	if err != nil {
		return []string{}, err
	}

	if len(aggrNames) == 1 {
		aggrName := aggrNames[0]
		if _, ok := CommonAggregatorsConst[aggrName]; !ok {
			return []string{}, fmt.Errorf("bad aggregator %s", aggrName)
		}
	} else {
		for _, aggrName := range aggrNames {
			if _, ok := HistogramAggregatorsConst[aggrName]; !ok {
				return []string{}, fmt.Errorf("bad aggregator %s", aggrName)
			}
		}
	}

	return aggrNames, nil
}

func (f Func) translateAggregator(aggr string) ([]string, error) {
	if len(aggr) == 0 {
		return nil, fmt.Errorf("emtpy aggr")
	}

	return strings.Split(aggr, ","), nil
}

// value line
// 拆解为子字符串, 根据协议不同, 每个协议单独对子串进行处理
func (f Func) TranslateValueLine(valueLine string) ([]string, error) {
	if len(valueLine) == 0 {
		return nil, fmt.Errorf("empty value line")
	}

	return strings.Split(valueLine, MergeDelimiter), nil
}

//
func (f Func) IsOk(code string) bool {
	if ok, exist := RpcOkCodesConst[code]; exist && ok {
		return true
	}
	return false
}

// 检查 a是否为b的keys的子集(subKeys)
func (f Func) IsSubKeys(a []string, b map[string]string) bool {
	isAllSub := true
	for i := 0; i < len(a) && isAllSub; i++ {
		isSub := false
		for k, _ := range b {
			if a[i] == k {
				isSub = true
				break
			}
		}
		if !isSub {
			isAllSub = false
		}
	}
	return isAllSub
}

// 检查 排序字符串数组数组 a中是否有完全相同的数组
func (f Func) HasSameSortedArray(a [][]string) bool {
	hasSameArray := false
	for i := 0; i < len(a) && !hasSameArray; i++ {
		for k := i + 1; k < len(a) && !hasSameArray; k++ {
			t1 := a[i]
			t2 := a[k]
			if len(t1) != len(t2) {
				continue
			}

			isEqualArray := true
			for j := 0; j < len(t1) && isEqualArray; j++ {
				if t1[j] != t2[j] {
					isEqualArray = false
				}
			}

			if isEqualArray {
				hasSameArray = true
			}
		}
	}

	return hasSameArray
}

// consts不能被修改, vars可以被修改
func (f Func) MergeSortedArrays(consts, vars [][]string) [][]string {
	for i := 0; i < len(consts); i++ {
		// check same
		hasSame := false
		for j := 0; j < len(vars) && !hasSame; j++ {
			if len(consts[i]) != len(vars[j]) {
				continue
			}
			isAllItemSame := true
			for k := 0; k < len(consts[i]) && isAllItemSame; k++ {
				if consts[i][k] != vars[j][k] {
					isAllItemSame = false
				}
			}
			if isAllItemSame {
				hasSame = true
			}
		}
		if !hasSame {
			vars = append(vars, consts[i])
		}
	}
	return vars
}

type TraceHandler struct {
	sync.RWMutex
	SecurityScanCounter map[string]float64 // map[ns]counter
}

var traceHandler = &TraceHandler{SecurityScanCounter: map[string]float64{}}

func (t *TraceHandler) rollHandler() *TraceHandler {
	t.Lock()
	defer t.Unlock()
	old := &TraceHandler{SecurityScanCounter: map[string]float64{}}
	old.SecurityScanCounter = t.SecurityScanCounter
	t.SecurityScanCounter = make(map[string]float64)
	return old
}

// 后续可以做很多, 比如打印日志,关联把脉 等
func (t *TraceHandler) collectAndIgnore(nsMetric string, traceid string) bool {
	t.Lock()
	defer t.Unlock()

	ignore := false
	if strings.HasSuffix(traceid, "ff") {
		ignore = true
		if _, found := t.SecurityScanCounter[nsMetric]; !found {
			t.SecurityScanCounter[nsMetric] = 1
		} else {
			t.SecurityScanCounter[nsMetric] += 1
		}
	}

	return ignore
}

// 不需要加锁, 单线程不会并发
func (t *TraceHandler) dumpPoints(reportTime time.Time) []*Point {
	var ret []*Point
	if len(t.SecurityScanCounter) == 0 {
		return ret
	}
	ts := reportTime.Unix()
	for nsMetric, counter := range t.SecurityScanCounter {
		slice := strings.Split(nsMetric, "/")
		if len(slice) != 2 {
			continue
		}
		ns := slice[0]
		if !strings.HasPrefix(ns, NsPrefixConst) {
			ns = NsPrefixConst + ns
		}
		ret = append(ret, &Point{
			Namespace: ns,
			Name:      "security.scan.counter",
			Timestamp: ts,
			Tags: map[string]string{
				"metric": slice[1],
			},
			Value: counter,
		})
	}
	return ret
}
