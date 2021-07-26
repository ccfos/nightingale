package feishu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type Result struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type feishuReqData struct {
	Msgtype string       `json:"msg_type"`
	Content *textContent `json:"content"`
}

type textContent struct {
	Text string `json:"text"`
}

// RobotSend robot发送信息
func RobotSend(tokenUser, sendContent string) error {
	url := "https://open.feishu.cn/open-apis/bot/v2/hook/" + tokenUser
	feishuReqData := new(feishuReqData)
	feishuReqData.Msgtype = "text"
	reqContent := new(textContent)
	reqContent.Text = sendContent
	feishuReqData.Content = reqContent

	content, err := json.Marshal(feishuReqData)
	if err != nil {
		return fmt.Errorf("feishu marshal req data err: %v", err)
	}

	data := bytes.NewReader(content)
	req, err := http.NewRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("feishu create new req err: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("feishu do req err: %v", err)
	}
	defer r.Body.Close()

	resp, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("feishu read req body err: %v", err)
	}

	result := Result{}
	err = json.Unmarshal(resp, &result)
	if err != nil {
		return fmt.Errorf("feishu unmarshal req content err: %v", err)
	}

	if result.ErrCode != 0 {
		err = fmt.Errorf("feishu req return ErrCode = %d ErrMsg = %s", result.ErrCode, result.ErrMsg)
	}

	return err
}

//func main() {
//	ret := RobotSend("xxx-xxxx-xxxx-xxx-xxx", "This is a test")
//	fmt.Println(ret)
//}
