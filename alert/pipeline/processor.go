package pipeline

import (
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// Processor 是处理器接口，所有处理器类型都需要实现此接口
type Processor interface {
	Init(settings interface{}) (Processor, error)          // 初始化配置
	Process(ctx *ctx.Context, event *models.AlertCurEvent) // 处理告警事件
}

// NewProcessorFn 创建处理器的函数类型
type NewProcessorFn func(settings interface{}) (Processor, error)

// 处理器注册表，存储各种类型处理器的构造函数
var processorRegister = map[string]NewProcessorFn{}

// // ProcessorTypes 存储所有支持的处理器类型
// var Processors map[int64]models.Processor

// func init() {
// 	Processors = make(map[int64]models.Processor)
// }

// RegisterProcessor 注册处理器类型
func RegisterProcessor(typ string, p Processor) {
	if _, found := processorRegister[typ]; found {
		return
	}
	processorRegister[typ] = p.Init
}

// GetProcessorByType 根据类型获取处理器实例
func GetProcessorByType(typ string, settings interface{}) (Processor, error) {
	typ = strings.TrimSpace(typ)
	fn, found := processorRegister[typ]
	if !found {
		return nil, fmt.Errorf("processor type %s not found", typ)
	}

	processor, err := fn(settings)
	if err != nil {
		return nil, err
	}

	return processor, nil
}
