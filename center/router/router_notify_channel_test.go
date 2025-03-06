package router

import (
	"fmt"
	"testing"
)

func TestGetFlashDutyChannels(t *testing.T) {
	// 构造测试数据
	integrationUrl := "https://api.flashcat.cloud/event/push/alert/n9e?integration_key=xxx"
	jsonData := []byte(`{}`)

	// 调用被测试的函数
	channels, err := getFlashDutyChannels(integrationUrl, jsonData)

	fmt.Println(channels, err)
}
