package httplib

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
)

// PostJSON 方法废弃，后面都使用beego的那些方法，beego的httplib做了改动，会复用transport
func PostJSON(url string, timeout time.Duration, v interface{}, headers map[string]string) (response []byte, code int, err error) {
	var bs []byte
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	bs, err = json.Marshal(v)
	if err != nil {
		return
	}

	bf := bytes.NewBuffer(bs)

	client := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest("POST", url, bf)
	req.Header.Set("Content-Type", "application/json")

	if headers != nil {
		for k, v := range headers {
			req.Header.Add(k, v)
		}
	}

	var resp *http.Response
	resp, err = client.Do(req)
	if err != nil {
		return
	}

	code = resp.StatusCode

	if resp.Body != nil {
		defer resp.Body.Close()
		response, err = ioutil.ReadAll(resp.Body)
	}

	return
}
