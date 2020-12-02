package dingtalk

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

type dingReqData struct {
	Msgtype string       `json:"msgtype"`
	Text    *textContent `json:"text"`
}

type textContent struct {
	Content string `json:"content"`
}

// RobotSend robot发送信息
func RobotSend(tokenUser, sendContent string) error {
	url := "https://oapi.dingtalk.com/robot/send?access_token=" + tokenUser

	dingReqData := new(dingReqData)
	dingReqData.Msgtype = "text"
	reqContent := new(textContent)
	reqContent.Content = sendContent
	dingReqData.Text = reqContent

	content, err := json.Marshal(dingReqData)
	if err != nil {
		return fmt.Errorf("dingtalk marshal req data err: %v", err)
	}

	data := bytes.NewReader(content)
	req, err := http.NewRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("dingtalk create new req err: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk do req err: %v", err)
	}
	defer r.Body.Close()

	resp, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("dingtalk read req body err: %v", err)
	}

	result := Result{}
	err = json.Unmarshal(resp, &result)
	if err != nil {
		return fmt.Errorf("dingtalk unmarshal req content err: %v", err)
	}

	if result.ErrCode != 0 {
		err = fmt.Errorf("dingtalk req return ErrCode = %d ErrMsg = %s", result.ErrCode, result.ErrMsg)
	}

	return err
}
