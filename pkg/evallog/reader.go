package evallog

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/toolkits/pkg/logger"
)

const (
	// DefaultQueryLimit 查询默认返回条数
	DefaultQueryLimit = 500
	// MaxQueryLimit 查询单次最大返回条数
	MaxQueryLimit = 2000

	// maxScanLineBytes 读取单行 JSONL 的扫描缓冲上限。
	// 写入侧的 maxRecordLineBytes 由它反推，两者必须成对修改。
	maxScanLineBytes = 4 * 1024 * 1024
	// maxRecordLineBytes 写入单行的硬上限，留出换行与编码余量
	maxRecordLineBytes = maxScanLineBytes - 1024
)

// truncHour 按**本地时区**把时间截到整点。
//
// 不能用 time.Truncate(time.Hour)：它按自零时刻起的绝对时长取整，不作用于展现形式，
// 而写入侧是按本地展现形式分桶的（t.Format("2006-01-02") + "/" + t.Format("15")）。
// 在 +05:30 / +05:45 / +03:30 / +09:30 / -03:30 这类非整点偏移的时区里，Truncate 的
// 结果会落在本地的 :30 或 :45 上，Format("15") 拿到的是上一个小时，于是每小时前半段
// 的记录永远定位不到文件——排障时会被读成「当时确实没数据」。
func truncHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}

// clampFromToRetention 把查询下界钳到保留期起点。
//
// 否则 from=0（引擎侧 handler 的历史默认值，也可由调用方显式传入）会让小时循环从
// 当前时刻一路倒退到 1970 年——约 49 万次迭代、近百万次 os.Open，单次请求即可占住
// handler 数秒，反复发起就是对告警引擎的慢速 DoS。保留期之外本就没有数据可读，
// 钳掉不损失任何结果。
func clampFromToRetention(cfg Config, fromMs int64, now time.Time) int64 {
	floor := now.Add(-time.Duration(cfg.RetentionHours) * time.Hour).UnixMilli()
	if fromMs < floor {
		return floor
	}
	return fromMs
}

// futureSkewAllowance 查询上界允许超出当前时刻的余量。
//
// 留 1 小时而不是几分钟：多出来的只有 1 轮小时循环（微秒级），却能覆盖前端 to 取自
// **浏览器时钟**（fe 侧是 moment(range.end).unix()）、NTP 跳变、时区配错导致的整小时
// 偏移这三类现实偏差。再放大就纯属遮蔽问题了——真出现比 now 大一天的 to，说明某个
// 时钟是坏的，早暴露比默默兼容好。
const futureSkewAllowance = time.Hour

// clampToToNow 把查询上界钳到「当前时刻 + futureSkewAllowance」。
//
// 与 clampFromToRetention 成对出现，缺一不可：下界钳住了、上界不钳，小时循环的轮数
// 依然由调用方随意指定的 to 决定。两个 handler 的 to 都不校验（center 侧是
// ginx.QueryInt64(c, "to", now) 之后 to*1000），传 to=253402300799（公元 9999 年）
// 就是约 7000 万轮、每轮 2 次 os.Open——实测约 4 分钟纯 syscall，具备 /alert-rules
// 权限的普通用户即可反复触发。两个钳制合起来，轮数被 RetentionHours 自动封顶
// （默认 192 轮，实测 0.63ms），因此不需要再单独设「最大查询跨度」。
//
// 记录的 Ts 取评估开始时刻，本就不存在未来数据，钳掉的全是空轮，不会藏数据。
func clampToToNow(toMs int64, now time.Time) int64 {
	ceil := now.Add(futureSkewAllowance).UnixMilli()
	if toMs > ceil {
		return ceil
	}
	return toMs
}

// QueryResult 一次查询的结果与容量说明。
//
// Truncated 不能省：字节预算生效时返回的条数会少于 limit，而前端判断「还有更多」用的是
// 「本页条数 == limit」，少一条就会被读成「这段时间就这么多记录」——本功能全篇都在防的
// 正是这种「容量上限被读成数据缺失」。所以预算一旦生效，必须带着可读的原因浮到接口上。
type QueryResult struct {
	Records   []EvalRecord
	Truncated bool
	Note      string // 人可读的截断原因，仅 Truncated 时非空
}

// queryRecords 扫描 [fromMs, toMs] 覆盖的小时文件，返回按 ts 倒序的记录。
// 小时文件按文件名直接定位，无需索引；beforeMs > 0 时作为翻页游标（ts < beforeMs）。
func queryRecords(cfg Config, ruleId, datasourceId int64, fromMs, toMs, beforeMs int64, limit int) (QueryResult, error) {
	if limit <= 0 {
		limit = DefaultQueryLimit
	}
	if limit > MaxQueryLimit {
		limit = MaxQueryLimit
	}
	now := time.Now()
	if toMs <= 0 {
		toMs = now.UnixMilli()
	}
	// 入参本身就倒挂属于调用方错误，先于任何钳制判定，保持原有报错语义
	if fromMs < 0 || fromMs > toMs {
		return QueryResult{}, fmt.Errorf("invalid time range: from %d > to %d", fromMs, toMs)
	}
	// 上下界成对钳制，把小时循环的轮数封顶在 RetentionHours 以内
	toMs = clampToToNow(toMs, now)
	fromMs = clampFromToRetention(cfg, fromMs, now)
	// 钳制之后才出现的倒挂（例如整个窗口都在未来）不是调用方错误，返回空即可
	empty := QueryResult{Records: []EvalRecord{}}
	if fromMs > toMs {
		return empty, nil
	}
	upper := toMs
	if beforeMs > 0 && beforeMs-1 < upper {
		upper = beforeMs - 1
	}
	if upper < fromMs {
		return empty, nil
	}

	ruleDir := filepath.Join(cfg.Dir, fmt.Sprintf("%d_%d", ruleId, datasourceId))
	if _, err := os.Stat(ruleDir); os.IsNotExist(err) {
		return empty, nil
	}

	budget := int64(cfg.MaxQueryBytes)
	if budget <= 0 {
		budget = defMaxQueryBytes
	}
	// budgetFloor 预算剩不下一条像样的记录时就收手：为了榨干最后这点余量去整读一个可能
	// 几十 MB 的小时文件并不划算，何况环形缓冲"至少留一条"的兜底还会让结果里多出一条
	// 孤零零的旧记录，看起来像是数据断续。
	budgetFloor := budget / 64
	res := QueryResult{Records: make([]EvalRecord, 0, 64)}
	// 环形缓冲整个查询只建一次、逐小时复用槽位，避免每小时都重新分配
	ring := newLineRing(limit)
	// 从最新小时向旧遍历，攒够 limit 或用光字节预算即止
	for h := truncHour(msTime(upper)); !h.Before(truncHour(msTime(fromMs))); h = h.Add(-time.Hour) {
		remaining := limit - len(res.Records)
		if remaining <= 0 {
			break
		}
		if budget <= budgetFloor {
			// 预算见底：更早的小时文件连打开都不打开。返回的是最新的那一段，
			// 丢的是更旧的，符合"先看最近发生了什么"的排障顺序
			res.Truncated = true
			break
		}
		// 把剩余 limit 与剩余字节预算一起下推到扫描环节：每个小时只留该小时内最新的
		// remaining 条、且总字节不超预算，常驻内存由预算决定，而不是由小时文件大小决定
		ring.reset(remaining, budget)
		if err := scanHourFile(ruleDir, h, fromMs, upper, ring); err != nil {
			return QueryResult{}, err
		}
		res.Records = ring.appendDesc(res.Records)
		budget -= ring.bytes
		if ring.dropped {
			res.Truncated = true
		}
	}
	if res.Truncated {
		res.Note = fmt.Sprintf("result truncated by the per-query byte budget ([Alert.EvalLog] MaxQueryBytes = %d): "+
			"only the newest %d records in this range are included; narrow the time range or lower limit",
			cfg.MaxQueryBytes, len(res.Records))
	}
	return res, nil
}

// tsProbe 只解出 ts 字段。区间外的行占绝大多数，对它们做整条反序列化纯属浪费。
type tsProbe struct {
	Ts int64 `json:"ts"`
}

// tsFieldPrefix EvalRecord.Ts 是结构体第一个字段，encoding/json 按声明顺序输出，
// 所以本包写出的每一行都以它开头。
const tsFieldPrefix = `{"ts":`

// probeTs 取出一行的 ts。
//
// 先走前缀快路径：热点是「把整个小时文件的每一行都过一遍」，而绝大多数行最终会因为
// 落在区间外、或被环形缓冲淘汰而丢掉，为它们做一次 json.Unmarshal（即使只解一个字段，
// 也要完整扫描整行 JSON）纯属浪费。快路径不匹配时（手工改过的文件、未来字段顺序调整）
// 退回通用解析，保证只快不错。
func probeTs(line []byte) (int64, bool) {
	if v, ok := fastProbeTs(line); ok {
		return v, true
	}
	var p tsProbe
	if err := json.Unmarshal(line, &p); err != nil {
		return 0, false
	}
	return p.Ts, true
}

func fastProbeTs(line []byte) (int64, bool) {
	if !bytes.HasPrefix(line, []byte(tsFieldPrefix)) {
		return 0, false
	}
	i := len(tsFieldPrefix)
	start := i
	var v int64
	for ; i < len(line); i++ {
		c := line[i]
		if c < '0' || c > '9' {
			break
		}
		// 毫秒时间戳 13 位，留到 18 位就足够，再长一律退回通用解析而不是悄悄溢出
		if i-start >= 18 {
			return 0, false
		}
		v = v*10 + int64(c-'0')
	}
	if i == start || i >= len(line) || (line[i] != ',' && line[i] != '}') {
		return 0, false
	}
	return v, true
}

// lineRing 定长环形缓冲，保留扫描过程中最后若干条命中记录的**原始行**。
//
// 小时文件内是时间升序，"最后 N 条"就是该小时里最新的 N 条，正是倒序结果需要的那段。
// 两处刻意的设计：
//
//   - 存原始行而不是解码后的 EvalRecord：进环的记录里只有最后 N 条会被真正返回，其余都
//     会被覆盖掉。原先对每条命中记录都做整条 json.Unmarshal（含 labels map 分配），解码
//     次数是 O(区间内记录数)；改成出环时才解码后变成 O(limit)，一个被写满的小时文件能少
//     掉几万次结构体分配，GC 压力直接落在告警引擎的评估延迟上。
//   - 除条数外还有字节上限：单条记录默认可到 176KB（100 曲线 × 60 点），前端一页取 1000
//     条，光是解码后的常驻就有百 MB 级，handler 再 json.Marshal 一份、center 侧还要再持有
//     一份——一次普通的翻页就能让告警引擎抖动。条数闸挡不住这个，必须按字节挡。
//
// 槽位底层数组跨小时复用（append(buf[i][:0], line...)），环写满后基本不再分配。
//
// 复用与「字节预算真的约束驻留」这两件事要靠 push 里的**换位**同时成立，不能只挪 start：
// 字节闸主导时 size 会长期停在远低于 maxLines 的水位，新元素落在 start+size 处、逐格前移，
// 若被淘汰的槽位原地不动，start 会走遍全部 maxLines 个槽位、每个各留一份大数组——
// 在册字节合规，sum(cap(buf[i])) 却能到预算的十倍（limit=2000、145KB 行、32MB 预算实测 296.9MB）。
// 因此淘汰时把腾出的数组换到下一个写入位：持有大数组的槽位数恒等于在册条数，且零重新分配。
type lineRing struct {
	buf      [][]byte
	maxLines int   // 本小时允许保留的条数，不超过 len(buf)
	maxBytes int64 // 本小时可用的字节预算
	bytes    int64 // 环内当前占用字节
	start    int   // 最旧元素下标
	size     int
	dropped  bool // 因字节预算淘汰过更旧的记录（条数淘汰不算：那是正常的 limit 语义）
}

func newLineRing(capacity int) *lineRing {
	if capacity < 1 {
		capacity = 1
	}
	return &lineRing{buf: make([][]byte, capacity), maxLines: capacity}
}

// reset 复用底层槽位开始新一轮（下一个小时文件）的收集。
func (r *lineRing) reset(maxLines int, maxBytes int64) {
	if maxLines < 1 {
		maxLines = 1
	}
	if maxLines > len(r.buf) {
		maxLines = len(r.buf)
	}
	// maxLines 随剩余 limit 单调收窄，超出的槽位这次查询内不会再被索引到
	//（下标运算全部 % maxLines），但可能还压着上一个小时留下的大数组，就地释放
	for i := maxLines; i < len(r.buf); i++ {
		r.buf[i] = nil
	}
	r.maxLines = maxLines
	r.maxBytes = maxBytes
	r.bytes = 0
	r.start = 0
	r.size = 0
	r.dropped = false
}

func (r *lineRing) push(line []byte) {
	n := int64(len(line))
	// 字节预算：淘汰更旧的直到放得下。环空了还放不下说明单条就超预算，
	// 那也要留下这一条——宁可超预算一条，也不能返回空列表让人以为没数据
	for r.size > 0 && r.bytes+n > r.maxBytes {
		r.bytes -= int64(len(r.buf[r.start]))
		// 把腾出的底层数组换到下一个写入位（start+size 在 start++/size-- 前后恒等），
		// 而不是留在原地：留在原地就是「持有大数组的槽位数一路涨到 maxLines」，
		// 字节预算只约束在册字节、约束不到实际驻留。换位后复用照旧，不产生新分配。
		dst := (r.start + r.size) % r.maxLines
		r.buf[r.start], r.buf[dst] = r.buf[dst], r.buf[r.start]
		r.start = (r.start + 1) % r.maxLines
		r.size--
		r.dropped = true
	}

	if r.size == r.maxLines {
		// 条数满：覆盖最旧的一条并前移起点
		idx := r.start
		r.bytes -= int64(len(r.buf[idx]))
		r.buf[idx] = append(r.buf[idx][:0], line...)
		r.bytes += n
		r.start = (r.start + 1) % r.maxLines
		return
	}

	idx := (r.start + r.size) % r.maxLines
	r.buf[idx] = append(r.buf[idx][:0], line...)
	r.bytes += n
	r.size++
}

// appendDesc 按时间倒序（最新在前）解码环内元素并追加到 dst。
func (r *lineRing) appendDesc(dst []EvalRecord) []EvalRecord {
	for i := r.size - 1; i >= 0; i-- {
		var rec EvalRecord
		if err := json.Unmarshal(r.buf[(r.start+i)%r.maxLines], &rec); err != nil {
			// probeTs 能过说明行首正常，整条解不出来只可能是尾部损坏，跳过即可
			continue
		}
		dst = append(dst, rec)
	}
	return dst
}

// scanHourFile 流式扫描某小时的记录文件，把 [fromMs, upperMs] 内的记录喂给 ring。
func scanHourFile(ruleDir string, hour time.Time, fromMs, upperMs int64, ring *lineRing) error {
	base := filepath.Join(ruleDir, hour.Format(dateLayout), hour.Format(hourLayout))

	// 顺序固定为 .gz 在前、.jsonl 在后：滚动压缩与写入存在短暂共存窗口，两个文件都要读，
	// 且 .gz 里的记录更早，先扫才能让 ring 内保持时间升序。
	for _, f := range []struct {
		path    string
		gzipped bool
	}{
		{base + ".jsonl.gz", true},
		{base + ".jsonl", false},
	} {
		if err := scanJsonlFile(f.path, f.gzipped, fromMs, upperMs, ring); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func scanJsonlFile(path string, gzipped bool, fromMs, upperMs int64, ring *lineRing) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var r io.Reader = f
	if gzipped {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("evallog gzip open %s: %v", path, err)
		}
		defer zr.Close()
		r = zr
	}

	matched := 0
	scanner := bufio.NewScanner(r)
	// 单条记录上限 256KB（默认），buffer 放宽到 maxScanLineBytes 以兼容配置放大的场景；
	// 写入侧的 enforceLineCeiling 保证不会写出超过这个长度的行
	scanner.Buffer(make([]byte, 64*1024), maxScanLineBytes)
	for scanner.Scan() {
		// scanner.Bytes() 指向扫描器内部缓冲，下一次 Scan 就会被覆盖；
		// ring.push 内部做拷贝（并复用槽位底层数组），这里不能自己再留引用
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		ts, ok := probeTs(line)
		if !ok {
			// 尾部半行（进程崩溃或并发追加中）跳过
			continue
		}
		if ts < fromMs || ts > upperMs {
			continue
		}
		ring.push(line)
		matched++
	}
	if err := scanner.Err(); err != nil {
		// 截断的 gzip 尾、超长行等异常：保留已扫到的部分而不是整体失败，
		// 但必须留下痕迹——静默返回半个文件会被当成「这段时间就这么多记录」
		logger.Warningf("evallog scan %s stopped early after %d matched records: %v", path, matched, err)
	}
	return nil
}
