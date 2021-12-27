package router

import (
	"compress/gzip"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/gin-gonic/gin"
)

type TimeSeries struct {
	Series []*Metric `json:"series"`
}

type Metric struct {
	Metric string   `json:"metric"`
	Points []Point  `json:"points"`
	Host   string   `json:"host"`
	Tags   []string `json:"tags,omitempty"`
}

type Point [2]float64

func datadogSeries(c *gin.Context) {
	apiKey, has := c.GetQuery("api_key")
	if !has {
		apiKey = ""
	}

	if len(config.C.BasicAuth) > 0 {
		// n9e-server need basic auth
		ok := false
		for _, v := range config.C.BasicAuth {
			if apiKey == v {
				ok = true
				break
			}
		}

		if !ok {
			c.String(http.StatusUnauthorized, "unauthorized")
			return
		}
	}

	var bs []byte
	var err error

	if c.GetHeader("Content-Encoding") == "gzip" {
		r, err := gzip.NewReader(c.Request.Body)
		if err != nil {
			c.String(400, err.Error())
			return
		}
		defer r.Close()
		bs, err = ioutil.ReadAll(r)
	} else {
		defer c.Request.Body.Close()
		bs, err = ioutil.ReadAll(c.Request.Body)
	}

	if err != nil {
		c.String(400, err.Error())
		return
	}

	var series TimeSeries
	err = json.Unmarshal(bs, &series)
	if err != nil {
		c.String(400, err.Error())
		return
	}

	// TODO clean and convert

}
