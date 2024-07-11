package flashduty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ccfos/nightingale/v6/center/cconf"

	"github.com/toolkits/pkg/logger"
)

var (
	Api     string
	Headers map[string]string
	Timeout time.Duration
)

func Init(fdConf cconf.FlashDuty) {
	Api = fdConf.Api
	Headers = make(map[string]string)
	Headers = fdConf.Headers

	if fdConf.Timeout == 0 {
		Timeout = 5 * time.Second
	} else {
		Timeout = fdConf.Timeout * time.Millisecond
	}
}

type dutyResp struct {
	RequestId string `json:"request_id"`
	Data      Data   `json:"data"`
	Error     struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type Data struct {
	P     int    `json:"p"`
	Limit int    `json:"limit"`
	Total int    `json:"total"`
	Items []Item `json:"items"`
}

type Item struct {
	MemberID      int    `json:"member_id"`
	MemberName    string `json:"member_name"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
}

func PostFlashDuty(path string, appKey string, body interface{}) error {
	_, err := PostFlashDutyWithResp(path, appKey, body)
	return err
}

func PostFlashDutyWithResp(path string, appKey string, body interface{}) (Data, error) {
	urlParams := url.Values{}
	urlParams.Add("app_key", appKey)
	var url string
	if Api != "" {
		url = fmt.Sprintf("%s%s?%s", Api, path, urlParams.Encode())
	} else {
		url = fmt.Sprintf("%s%s?%s", "https://api.flashcat.cloud", path, urlParams.Encode())
	}
	response, code, err := PostJSON(url, Timeout, Headers, body)
	req, _ := json.Marshal(body)
	logger.Infof("flashduty post: url=%s, req=%s; response=%s, code=%d", url, string(req), string(response), code)

	var resp dutyResp
	if err == nil {
		e := json.Unmarshal(response, &resp)
		if e == nil && resp.Error.Message != "" {
			err = fmt.Errorf("flashduty post error: %s", resp.Error.Message)
		}
	}

	return resp.Data, err
}

func PostJSON(url string, timeout time.Duration, headers map[string]string, v interface{}, retries ...int) (response []byte, code int, err error) {
	var bs []byte

	bs, err = json.Marshal(v)
	if err != nil {
		return
	}

	bf := bytes.NewBuffer(bs)

	client := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest("POST", url, bf)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	if len(headers) > 0 {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	var resp *http.Response

	if len(retries) > 0 {
		for i := 0; i < retries[0]; i++ {
			resp, err = client.Do(req)
			if err == nil {
				break
			}

			tryagain := ""
			if i+1 < retries[0] {
				tryagain = " try again"
			}

			logger.Warningf("failed to curl %s error: %s"+tryagain, url, err)

			if i+1 < retries[0] {
				time.Sleep(time.Millisecond * 200)
			}
		}
	} else {
		resp, err = client.Do(req)
	}

	if err != nil {
		return
	}

	code = resp.StatusCode

	if resp.Body != nil {
		defer resp.Body.Close()
		response, err = io.ReadAll(resp.Body)
	}

	return
}
