package poster

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
)

type DataResponse[T any] struct {
	Dat T      `json:"dat"`
	Err string `json:"err"`
}

func GetByUrls[T any](ctx *ctx.Context, path string) (T, error) {
	var err error
	addrs := ctx.CenterApi.Addrs

	rand.Shuffle(len(addrs), func(i, j int) { addrs[i], addrs[j] = addrs[j], addrs[i] })
	for _, addr := range addrs {
		url := fmt.Sprintf("%s%s", addr, path)
		dat, e := GetByUrl[T](url, ctx.CenterApi.BasicAuth)
		if e != nil {
			err = e
			logger.Warningf("failed to get data from center, url: %s, err: %v", url, err)
			continue
		}
		return dat, nil
	}

	var dat T
	return dat, err
}

func GetByUrl[T any](url string, basicAuth gin.Accounts) (T, error) {
	var dat T
	req := httplib.Get(url).SetTimeout(time.Duration(3000) * time.Millisecond)

	if len(basicAuth) > 0 {
		var token string
		for username, password := range basicAuth {
			token = base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		}

		if len(token) > 0 {
			req = req.Header("Authorization", "Basic "+token)
		}
	}

	resp, err := req.Response()
	if err != nil {
		return dat, fmt.Errorf("failed to fetch from url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return dat, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return dat, fmt.Errorf("failed to read response body: %w", err)
	}

	var dataResp DataResponse[T]
	err = json.Unmarshal(body, &dataResp)
	if err != nil {
		return dat, fmt.Errorf("failed to decode response: %w", err)
	}

	if dataResp.Err != "" {
		return dat, fmt.Errorf("error from server: %s", dataResp.Err)
	}

	logger.Debugf("get data from %s, data: %+v", url, dataResp.Dat)
	return dataResp.Dat, nil
}

type PostResponse struct {
	Err string `json:"err"`
}

func PostByUrls(ctx *ctx.Context, path string, v interface{}) (err error) {
	addrs := ctx.CenterApi.Addrs

	rand.Shuffle(len(addrs), func(i, j int) { addrs[i], addrs[j] = addrs[j], addrs[i] })
	for _, addr := range addrs {
		url := fmt.Sprintf("%s%s", addr, path)
		err = PostByUrl(url, ctx.CenterApi.BasicAuth, v)
		if err == nil {
			return
		}
	}
	return
}

func PostByUrl(url string, basicAuth gin.Accounts, v interface{}) (err error) {
	var bs []byte
	bs, err = json.Marshal(v)
	if err != nil {
		return
	}
	bf := bytes.NewBuffer(bs)
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("POST", url, bf)
	req.Header.Set("Content-Type", "application/json")

	if len(basicAuth) > 0 {
		var token string
		for username, password := range basicAuth {
			token = base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		}
		if len(token) > 0 {
			req.Header.Set("Authorization", "Basic "+token)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch from url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var dataResp PostResponse
	err = json.Unmarshal(body, &dataResp)
	if err != nil {
		return fmt.Errorf("failed to decode response: %w body:%s", err, string(body))
	}

	if dataResp.Err != "" {
		return fmt.Errorf("error from server: %s", dataResp.Err)
	}

	return nil
}

func PostJSON(url string, timeout time.Duration, v interface{}, retries ...int) (response []byte, code int, err error) {
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
	req.Header.Set("Content-Type", "application/json")

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
		response, err = ioutil.ReadAll(resp.Body)
	}

	return
}
