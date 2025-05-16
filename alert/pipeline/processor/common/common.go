package common

import (
	"encoding/json"
)

// InitProcessor 是一个通用的初始化处理器的方法
// 使用泛型简化处理器初始化逻辑
// T 必须是 models.Processor 接口的实现
func InitProcessor[T any](settings interface{}) (T, error) {
	var zero T
	b, err := json.Marshal(settings)
	if err != nil {
		return zero, err
	}

	var result T
	err = json.Unmarshal(b, &result)
	if err != nil {
		return zero, err
	}

	return result, nil
}
