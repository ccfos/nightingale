package datasource

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// 告警规则的「附加查询」在前端只存 sql + interval + offset，不存 from/to，
// 查询窗口由插件在执行时算出来。不补窗口的话，SQL 里的 $__timeFilter 会按 1970 年展开，
// 规则存得下但查不出数据。interval 没配（或配成 0）时按这个跨度查，单位秒。
const DefaultEnrichInterval int64 = 60

// NormalizeQueryTimeRange 按 interval/offset 补齐 SQL 数据源 QueryLog 的时间窗口，规则与 doris 的 QueryLog 一致：
// 没传 from/to 且带了 interval 时补成 [now-interval, now]；offset 不为 0 时整个窗口往前挪。
//
// 放在 QueryLog 里而不是只放在 QueryMapData 里，是为了让附加查询的预览（传选择器的 from/to 加 offset）
// 和告警时刻的执行应用同一个 offset。仪表盘、探索页的请求带 from/to、不带 offset，不受影响。
func NormalizeQueryTimeRange(from, to, interval int64, offset int) (int64, int64) {
	if from == 0 && to == 0 && interval > 0 {
		to = time.Now().Unix()
		from = to - interval
	}

	if offset != 0 {
		from -= int64(offset)
		to -= int64(offset)
	}

	return from, to
}

// RowsToStringMaps 把 SQL 数据源 QueryLog 返回的原始行转成 QueryMapData 要的形态。
//
// 值为 nil 的列直接丢掉，整行为空的行也丢掉。
func RowsToStringMaps(items []interface{}) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		m := make(map[string]string, len(row))
		for k, v := range row {
			if v == nil {
				continue
			}
			m[k] = stringifyColumnValue(v)
		}

		if len(m) > 0 {
			rows = append(rows, m)
		}
	}
	return rows
}

// stringifyColumnValue 把单个列值转成展示用的字符串。
//
// 数据库驱动返回的时间列是 time.Time、数值列是各种宽度的整型/浮点，交给 json.Marshal
// 会得到带引号的 RFC3339 串和科学计数法，通知里很难看，所以这几类单独处理。
func stringifyColumnValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case time.Time:
		return val.Format("2006-01-02 15:04:05")
	case bool:
		return strconv.FormatBool(val)
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
