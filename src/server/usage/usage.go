package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/version"
	"github.com/didi/nightingale/v5/src/server/common/conv"
	"github.com/didi/nightingale/v5/src/server/reader"
)

const (
	url     = "http://n9e.io/report"
	request = "sum(rate(n9e_server_samples_received_total[5m]))"
)

type Usage struct {
	Samples    float64 `json:"samples"` // per second
	Users      float64 `json:"users"`   // user total
	Maintainer string  `json:"maintainer"`
	Hostname   string  `json:"hostname"`
	Version    string  `json:"version"`
}

func getSamples() (float64, error) {
	value, warns, err := reader.Client.Query(context.Background(), request, time.Now())
	if err != nil {
		return 0, err
	}

	if len(warns) > 0 {
		return 0, fmt.Errorf("occur some warnings: %v", warns)
	}

	lst := conv.ConvertVectors(value)
	if len(lst) == 0 {
		return 0, fmt.Errorf("convert result is empty")
	}

	return lst[0].Value, nil
}

func Report() {
	for {
		time.Sleep(time.Minute * 10)
		report()
	}
}

func report() {
	sps, err := getSamples()
	if err != nil {
		return
	}

	hostname, err := os.Hostname()
	if err != nil {
		return
	}

	num, err := models.UserTotal("")
	if err != nil {
		return
	}

	maintainer := "blank"

	u := Usage{
		Samples:    sps,
		Users:      float64(num),
		Hostname:   hostname,
		Maintainer: maintainer,
		Version:    version.VERSION,
	}

	post(u)
}

func post(u Usage) error {
	body, err := json.Marshal(u)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	cli := http.Client{
		Timeout: time.Second * 10,
	}

	resp, err := cli.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("got %s", resp.Status)
	}

	_, err = ioutil.ReadAll(resp.Body)
	return err
}
