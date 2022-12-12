package main

import (
	"fmt"
	"time"

	"github.com/tidwall/gjson"
)

// the caller can be called for alerting notify by complete this interface
type inter interface {
	Descript() string
	Notify([]byte)
}

// N9E complete
type N9EPlugin struct {
	Name        string
	Description string
	BuildAt     string
}

func (n *N9EPlugin) Descript() string {
	return fmt.Sprintf("%s: %s", n.Name, n.Description)
}

func (n *N9EPlugin) Notify(bs []byte) {
	var channels = []string{
		"dingtalk_robot_token",
		"wecom_robot_token",
		"feishu_robot_token",
		"telegram_robot_token",
	}
	for _, ch := range channels {
		if ret := gjson.GetBytes(bs, ch); ret.Exists() {
			fmt.Printf("do something...")
		}
	}
}

// will be loaded for alertingCall , The first letter must be capitalized to be exported
var N9eCaller = N9EPlugin{
	Name:        "n9e",
	Description: "演示告警通过动态链接库方式通知",
	BuildAt:     time.Now().Local().Format("2006/01/02 15:04:05"),
}
