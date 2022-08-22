package usage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/version"
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

func Report() {
	for {
		time.Sleep(time.Minute * 10)
		report()
	}
}

func report() {
	tnum, err := models.TargetTotalCount()
	if err != nil {
		return
	}

	hostname, err := os.Hostname()
	if err != nil {
		return
	}

	unum, err := models.UserTotal("")
	if err != nil {
		return
	}

	maintainer := "blank"

	u := Usage{
		Samples:    float64(tnum),
		Users:      float64(unum),
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
