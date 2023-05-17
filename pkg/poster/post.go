package poster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

type DataResponse[T any] struct {
	Dat T      `json:"dat"`
	Err string `json:"err"`
}

func GetByUrls[T any](ctx *ctx.Context, path string) (T, error) {
	var err error
	addrs := ctx.Addrs
	rand.Shuffle(len(addrs), func(i, j int) { addrs[i], addrs[j] = addrs[j], addrs[i] })
	for _, addr := range addrs {
		url := fmt.Sprintf("%s%s", addr, path)
		dat, err := GetByUrl[T](url)
		if err == nil {
			return dat, nil
		}
	}

	var dat T
	return dat, err
}

func GetByUrl[T any](url string) (T, error) {
	var dat T

	resp, err := http.Get(url)
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

	return dataResp.Dat, nil
}

func PostByUrls(urls []string, timeout time.Duration, v interface{}) (response []byte, code int, err error) {
	rand.Shuffle(len(urls), func(i, j int) { urls[i], urls[j] = urls[j], urls[i] })
	for _, url := range urls {
		response, code, err = PostJSON(url, timeout, v)
		if err == nil {
			return
		}
	}
	return
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
