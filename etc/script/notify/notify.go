package main

import (
	"fmt"
	"time"

	"github.com/tidwall/gjson"
)

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

func (n *N9EPlugin) NotifyMaintainer(bs []byte) {
	fmt.Println("do something... begin")
	result := string(bs)
	fmt.Println(result)
	fmt.Println("do something... end")
}

// will be loaded for alertingCall , The first letter must be capitalized to be exported
var N9eCaller = N9EPlugin{
	Name:        "N9EPlugin",
	Description: "Notify by lib",
	BuildAt:     time.Now().Local().Format("2006/01/02 15:04:05"),
}
