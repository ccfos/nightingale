package models

import (
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type Processor interface {
	Init(settings interface{}) (Processor, error) // 初始化配置
	Process(ctx *ctx.Context, event *AlertCurEvent) (*AlertCurEvent, string, error)
	// 处理器有三种情况：
	// 1. 处理成功，返回处理后的事件
	// 2. 处理成功，不需要返回处理后端事件，只返回处理结果，将处理结果放到 string 中，比如 eventdrop callback 处理器
	// 3. 处理失败，返回错误，将错误放到 error 中
}

type NewProcessorFn func(settings interface{}) (Processor, error)

var processorRegister = map[string]NewProcessorFn{}

func RegisterProcessor(typ string, p Processor) {
	if _, found := processorRegister[typ]; found {
		return
	}
	processorRegister[typ] = p.Init
}

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
