package http_response

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/didi/nightingale/v4/src/common/i18n"
	"github.com/didi/nightingale/v4/src/modules/server/collector"
	"github.com/didi/nightingale/v4/src/modules/server/plugins"
	"github.com/didi/nightingale/v4/src/modules/server/plugins/http_response/http_response"
	"github.com/influxdata/telegraf"
)

func init() {
	collector.CollectorRegister(NewCollector()) // for monapi
	i18n.DictRegister(langDict)
}

type Collector struct {
	*collector.BaseCollector
}

func NewCollector() *Collector {
	return &Collector{BaseCollector: collector.NewBaseCollector(
		"http_response",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &Rule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"URLS":                             "地址",
			"Method":                           "方法",
			"ResponseTimeout":                  "响应超时",
			"Headers":                          "Headers",
			"Username":                         "用户名",
			"Password":                         "密码",
			"Body":                             "Body",
			"ResponseBodyMaxSize":              "ResponseBodyMaxSize",
			"ResponseStringMatch":              "ResponseStringMatch",
			"ResponseStatusCode":               "ResponseStatusCode",
			"Interface":                        "Interface",
			"HTTPProxy":                        "HTTPProxy",
			"FollowRedirects":                  "FollowRedirects",
			"List of urls to query":            "要监测的URL地址",
			"HTTP Request Method, default GET": "HTTP 的请求方法，默认是 GET",
			"HTTP Request Headers":             "HTTP 请求的的 Headers",
			"Optional HTTP Basic Auth Credentials, Username":                                   "HTTP Basic 认证的用户名",
			"Optional HTTP Basic Auth Credentials, Password":                                   "HTTP Basic 认证的密码",
			"Optional HTTP Request Body":                                                       "HTTP 请求的 Body",
			"If the response body size exceeds this limit a body_read_error will be raised":    "如果返回的 body 超过了限制，则会上报 body_read_error 对应的 result_code",
			"Optional substring or regex match in body of the response":                        "返回的 Body 中匹配的字符串，可以部分匹配或者正则",
			"Expected response status code, If match response_status_code_match will be 1":     "期望返回的状态码，如果匹配则 response_status_code_match 回上报 1",
			"Interface to use when dialing an address":                                         "发起请求使用的接口",
			"Set http_proxy (telegraf uses the system wide proxy settings if it's is not set)": "HTTP 代理的地址",
			"Whether to follow redirects from the server (defaults to false)":                  "是否自动跳转",
		},
	}
)

type Rule struct {
	URLs                []string `label:"URLs" json:"urls,required" description:"List of urls to query" example:"https://github.com/didi/nightingale"`
	Method              string   `label:"Method" json:"method" description:"HTTP Request Method, default GET" example:"GET"`
	ResponseTimeout     int      `label:"ResponseTimeout" json:"response_timeout" default:"5" description:"Set response_timeout (default 5 seconds)"`
	Headers             []string `label:"Headers" json:"headers" description:"HTTP Request Headers" example:"Content-Type: application/json"`
	Username            string   `label:"Username" json:"username" description:"Optional HTTP Basic Auth Credentials, Username" example:"username"`
	Password            string   `label:"Password" json:"password" description:"Optional HTTP Basic Auth Credentials, Password" example:"password"`
	Body                string   `label:"Body" json:"body" description:"Optional HTTP Request Body" example:"{'fake':'data'}"`
	ResponseBodyMaxSize int      `label:"ResponseBodyMaxSize" json:"response_body_max_size" default:"32" description:"If the response body size exceeds this limit a body_read_error will be raised"`
	ResponseStringMatch string   `label:"ResponseStringMatch" json:"response_string_match" description:"Optional substring or regex match in body of the response" example:"ok"`
	ResponseStatusCode  int      `label:"ResponseStatusCode" json:"response_status_code" default:"200" description:"Expected response status code, If match response_status_code_match will be 1"`
	Interface           string   `label:"Interface" json:"interface" description:"Interface to use when dialing an address" example:"eth0"`
	HTTPProxy           string   `label:"HTTPProxy" json:"http_proxy" description:"Set http_proxy (telegraf uses the system wide proxy settings if it's is not set)" example:"http://localhost:8888"`
	FollowRedirects     bool     `label:"FollowRedirects" json:"follow_redirects" description:"Whether to follow redirects from the server (defaults to false)"`
	plugins.ClientConfig
}

func checkHTTPMethod(method string) bool {
	httpMethods := []string{"GET", "HEAD", "POST", "OPTIONS", "PUT", "DELETE", "TRACE", "CONNECT"}
	for _, m := range httpMethods {
		if m == method {
			return true
		}
	}
	return false
}

func getHeaderMap(headers []string) (map[string]string, error) {
	headerMap := make(map[string]string)
	for _, header := range headers {
		kv := strings.Split(header, ":")
		if len(kv) != 2 {
			err := errors.New("header is not valid")
			return nil, err
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		headerMap[k] = v
	}
	return headerMap, nil
}

func (p *Rule) Validate() error {
	if len(p.URLs) == 0 || p.URLs[0] == "" {
		return fmt.Errorf("http_response.rule.urls must be set")
	}
	if p.Method == "" {
		p.Method = "GET"
	}
	if !checkHTTPMethod(p.Method) {
		return fmt.Errorf("http_response.rule.method is not valid")
	}
	if p.ResponseTimeout == 0 {
		p.ResponseTimeout = 5
	}
	if p.ResponseBodyMaxSize == 0 {
		p.ResponseBodyMaxSize = 32
	}

	return nil
}

func (p *Rule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	headerMap, err := getHeaderMap(p.Headers)
	if err != nil {
		return nil, err
	}

	input := &http_response.HTTPResponse{
		URLs:                p.URLs,
		Method:              p.Method,
		Username:            p.Username,
		Password:            p.Password,
		Headers:             headerMap,
		Body:                p.Body,
		ResponseStringMatch: p.ResponseStringMatch,
		ResponseStatusCode:  p.ResponseStatusCode,
		Interface:           p.Interface,
		HTTPProxy:           p.HTTPProxy,
		FollowRedirects:     p.FollowRedirects,
		Log:                 plugins.GetLogger(),
		ClientConfig:        p.ClientConfig.TlsClientConfig(),
	}
	if err := plugins.SetValue(&input.ResponseTimeout.Duration, time.Second*time.Duration(p.ResponseTimeout)); err != nil {
		return nil, err
	}
	if err := plugins.SetValue(&input.ResponseBodyMaxSize.Size, int64(p.ResponseBodyMaxSize)*1024*1024); err != nil {
		return nil, err
	}
	return input, nil
}
