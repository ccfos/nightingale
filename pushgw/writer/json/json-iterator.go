package json

import (
	"math"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
)

func init() {
	// 为了处理prom数据中的NaN值
	jsoniter.RegisterTypeEncoderFunc("float64", func(ptr unsafe.Pointer, stream *jsoniter.Stream) {
		f := *(*float64)(ptr)
		if math.IsNaN(f) {
			stream.WriteString("null")
		} else {
			stream.WriteFloat64(f)
		}
	}, func(ptr unsafe.Pointer) bool {
		return true
	})
}

func MarshalWithCustomFloat(items interface{}) ([]byte, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	return json.Marshal(items)
}
