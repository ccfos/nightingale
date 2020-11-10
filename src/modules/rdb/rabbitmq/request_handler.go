package rabbitmq

import (
	"encoding/json"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/models"
)

type MQRequest struct {
	Method  string      `json:"method"`
	Payload interface{} `json:"payload"`
}

// 返回的bool值代表是否让上层给mq发送ack
func handleMessage(msgBody []byte) bool {
	if len(msgBody) <= 0 {
		logger.Warning("msg body is blank")
		// 这是个异常消息，需要ack并丢弃
		return true
	}

	var req MQRequest
	err := json.Unmarshal(msgBody, &req)
	if err != nil {
		logger.Warning("unmarshal msg body fail")
		return true
	}

	if req.Method == "" {
		logger.Warning("mq_request.method is blank")
		return true
	}

	logger.Infof("mq_request, method: %s, payload: %v", req.Method, req.Payload)

	jsonBytes, err := json.Marshal(req.Payload)
	if err != nil {
		logger.Warning("mq_request.payload marshal fail: ", err)
		return true
	}

	err = dispatchHandler(req.Method, jsonBytes)
	if err != nil {
		// 如果处理的有问题，可能是后端DB挂了，不能ack，等DB恢复了还可以继续处理
		return false
	}

	return true
}

func dispatchHandler(method string, jsonBytes []byte) error {
	switch method {
	case "oplog_add":
		return oplogAdd(jsonBytes)
	case "res_create":
		return resourceRegister(jsonBytes)
	case "res_delete":
		return resourceUnregister(jsonBytes)
	default:
		logger.Warning("mq_request.method not support")
		return nil
	}
}

// 第三方系统通过MQ把操作日志推给RDB保存
func oplogAdd(jsonBytes []byte) error {
	var ol models.OperationLog
	err := json.Unmarshal(jsonBytes, &ol)
	if err != nil {
		// 传入的数据不合理，无法decode，这种数据要被消费丢掉
		logger.Error("cannot unmarshal OperationLog: ", err)
		return nil
	}

	return ol.New()
}

// 第三方系统，比如RDS、Redis等，资源创建了，要注册到RDB
func resourceRegister(jsonBytes []byte) error {
	var item models.ResourceRegisterItem
	err := json.Unmarshal(jsonBytes, &item)
	if err != nil {
		logger.Warning(err)
		return nil
	}

	errCode, err := models.ResourceRegisterFor3rd(item)
	if errCode == 0 {
		return nil
	}

	if errCode == 400 {
		logger.Warningf("item invalid: %v", err)
		return nil
	}

	// errCode == 500
	logger.Errorf("system internal error: %v", err)
	return err
}

// 第三方系统，比如RDS、Redis等，资源销毁了，要通知到RDB
func resourceUnregister(jsonBytes []byte) error {
	var item models.ResourceRegisterItem
	err := json.Unmarshal(jsonBytes, &item)
	if err != nil {
		logger.Warning(err)
		return nil
	}

	if item.UUID == "" {
		return nil
	}

	err = models.ResourceUnregister([]string{item.UUID})
	if err != nil {
		logger.Error(err)
		return err
	}

	return nil
}
