package models

import (
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type Processor interface {
	Init(settings interface{}) (Processor, error)   // 初始化配置
	Process(ctx *ctx.Context, event *AlertCurEvent) // 处理告警事件
}

type NewProcessorFn func(settings interface{}) (Processor, error)

// 处理器注册表，存储各种类型处理器的构造函数
var processorRegister = map[string]NewProcessorFn{}

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
