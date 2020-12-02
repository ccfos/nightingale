package wechat

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// Err 微信返回错误
type Err struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// AccessToken 微信企业号请求Token
type AccessToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Err
	ExpiresInTime time.Time
}

// Client 微信企业号应用配置信息
type Client struct {
	CorpID      string
	AgentID     int
	AgentSecret string
	Token       AccessToken
}

// Result 发送消息返回结果
type Result struct {
	Err
	InvalidUser  string `json:"invaliduser"`
	InvalidParty string `json:"infvalidparty"`
	InvalidTag   string `json:"invalidtag"`
}

// Content 文本消息内容
type Content struct {
	Content string `json:"content"`
}

// Message 消息主体参数
type Message struct {
	ToUser  string  `json:"touser"`
	ToParty string  `json:"toparty"`
	ToTag   string  `json:"totag"`
	MsgType string  `json:"msgtype"`
	AgentID int     `json:"agentid"`
	Text    Content `json:"text"`
}

// New 实例化微信企业号应用
func New(corpID string, agentID int, agentSecret string) *Client {
	c := new(Client)
	c.CorpID = corpID
	c.AgentID = agentID
	c.AgentSecret = agentSecret
	return c
}

// Send 发送信息
func (c *Client) Send(msg Message) error {
	if err := c.GetAccessToken(); err != nil {
		return err
	}

	msg.AgentID = c.AgentID
	url := "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=" + c.Token.AccessToken

	resultByte, err := jsonPost(url, msg)
	if err != nil {
		return fmt.Errorf("invoke send api fail: %v", err)
	}

	result := Result{}
	err = json.Unmarshal(resultByte, &result)
	if err != nil {
		return fmt.Errorf("parse send api response fail: %v", err)
	}

	if result.ErrCode != 0 {
		err = fmt.Errorf("invoke send api return ErrCode = %d", result.ErrCode)
	}

	if result.InvalidUser != "" || result.InvalidParty != "" || result.InvalidTag != "" {
		err = fmt.Errorf("invoke send api partial fail, invalid user: %s, invalid party: %s, invalid tag: %s", result.InvalidUser, result.InvalidParty, result.InvalidTag)
	}

	return err
}

// GetAccessToken 获取会话token
func (c *Client) GetAccessToken() error {
	var err error

	if c.Token.AccessToken == "" || c.Token.ExpiresInTime.Before(time.Now()) {
		c.Token, err = getAccessTokenFromWeixin(c.CorpID, c.AgentSecret)
		if err != nil {
			return fmt.Errorf("invoke getAccessTokenFromWeixin fail: %v", err)
		}
		c.Token.ExpiresInTime = time.Now().Add(time.Duration(c.Token.ExpiresIn-1000) * time.Second)
	}

	return err
}

// transport 全局复用，提升性能
var transport = &http.Transport{
	TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
	DisableCompression: true,
}

// getAccessTokenFromWeixin 从微信服务器获取token
func getAccessTokenFromWeixin(corpID, secret string) (accessToken AccessToken, err error) {
	url := "https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=" + corpID + "&corpsecret=" + secret

	client := &http.Client{Transport: transport}
	result, err := client.Get(url)
	if err != nil {
		return accessToken, fmt.Errorf("invoke api gettoken fail: %v", err)
	}

	if result.Body == nil {
		return accessToken, fmt.Errorf("gettoken response body is nil")
	}

	defer result.Body.Close()

	res, err := ioutil.ReadAll(result.Body)
	if err != nil {
		return accessToken, fmt.Errorf("read gettoken response body fail: %v", err)
	}

	err = json.Unmarshal(res, &accessToken)
	if err != nil {
		return accessToken, fmt.Errorf("parse gettoken response body fail: %v", err)
	}

	if accessToken.ExpiresIn == 0 || accessToken.AccessToken == "" {
		err = fmt.Errorf("invoke api gettoken fail, ErrCode: %v, ErrMsg: %v", accessToken.ErrCode, accessToken.ErrMsg)
		return accessToken, err
	}

	return accessToken, err
}

func jsonPost(url string, data interface{}) ([]byte, error) {
	jsonBody, err := encodeJSON(data)
	if err != nil {
		return nil, err
	}

	r, err := http.Post(url, "application/json;charset=utf-8", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	if r.Body == nil {
		return nil, fmt.Errorf("response body of %s is nil", url)
	}

	defer r.Body.Close()

	return ioutil.ReadAll(r.Body)
}

func encodeJSON(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RobotSend robot发送信息
func RobotSend(msg Message) error {
	url := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=" + msg.ToUser

	resultByte, err := jsonPost(url, msg)
	if err != nil {
		return fmt.Errorf("invoke send api fail: %v", err)
	}

	result := Result{}
	err = json.Unmarshal(resultByte, &result)
	if err != nil {
		return fmt.Errorf("parse send api response fail: %v", err)
	}

	if result.ErrCode != 0 {
		err = fmt.Errorf("invoke send api return ErrCode = %d", result.ErrCode)
	}

	if result.InvalidUser != "" || result.InvalidParty != "" || result.InvalidTag != "" {
		err = fmt.Errorf("invoke send api partial fail, invalid user: %s, invalid party: %s, invalid tag: %s", result.InvalidUser, result.InvalidParty, result.InvalidTag)
	}

	return err
}
