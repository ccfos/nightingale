package router

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/server/common"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/idents"
	"github.com/didi/nightingale/v5/src/server/memsto"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
	"github.com/didi/nightingale/v5/src/server/writer"
)

type HTTPMetric struct {
	Metric       string            `json:"metric"`
	Timestamp    int64             `json:"timestamp"`
	ValueUnTyped interface{}       `json:"value"`
	Value        float64           `json:"-"`
	Tags         map[string]string `json:"tags"`
}

func (m *HTTPMetric) Clean(ts int64) error {
	if m.Metric == "" {
		return fmt.Errorf("metric is blank")
	}

	switch v := m.ValueUnTyped.(type) {
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m.Value = f
		} else {
			return fmt.Errorf("unparseable value %v", v)
		}
	case float64:
		m.Value = v
	case uint64:
		m.Value = float64(v)
	case int64:
		m.Value = float64(v)
	case int:
		m.Value = float64(v)
	default:
		return fmt.Errorf("unparseable value %v", v)
	}

	// if timestamp bigger than 32 bits, likely in milliseconds
	if m.Timestamp > 0xffffffff {
		m.Timestamp /= 1000
	}

	// If the timestamp is greater than 5 minutes, the current time shall prevail
	diff := m.Timestamp - ts
	if diff > 300 {
		m.Timestamp = ts
	}
	return nil
}

func (m *HTTPMetric) ToProm() (*prompb.TimeSeries, error) {
	pt := &prompb.TimeSeries{}
	pt.Samples = append(pt.Samples, prompb.Sample{
		// use ms
		Timestamp: m.Timestamp * 1000,
		Value:     m.Value,
	})

	if strings.IndexByte(m.Metric, '.') != -1 {
		m.Metric = strings.ReplaceAll(m.Metric, ".", "_")
	}

	if strings.IndexByte(m.Metric, '-') != -1 {
		m.Metric = strings.ReplaceAll(m.Metric, "-", "_")
	}

	if !model.MetricNameRE.MatchString(m.Metric) {
		return nil, fmt.Errorf("invalid metric name: %s", m.Metric)
	}

	pt.Labels = append(pt.Labels, &prompb.Label{
		Name:  model.MetricNameLabel,
		Value: m.Metric,
	})

	if _, exists := m.Tags["ident"]; !exists {
		// rename tag key
		host, has := m.Tags["host"]
		if has {
			delete(m.Tags, "host")
			m.Tags["ident"] = host
		}
	}

	for key, value := range m.Tags {
		if strings.IndexByte(key, '.') != -1 {
			key = strings.ReplaceAll(key, ".", "_")
		}

		if strings.IndexByte(key, '-') != -1 {
			key = strings.ReplaceAll(key, "-", "_")
		}

		if !model.LabelNameRE.MatchString(key) {
			return nil, fmt.Errorf("invalid tag name: %s", key)
		}

		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  key,
			Value: value,
		})
	}

	return pt, nil
}

func handleOpenTSDB(c *gin.Context) {
	var bs []byte
	var err error
	var r *gzip.Reader

	if c.GetHeader("Content-Encoding") == "gzip" {
		r, err = gzip.NewReader(c.Request.Body)
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

	var arr []HTTPMetric

	if bs[0] == '[' {
		err = json.Unmarshal(bs, &arr)
	} else {
		var one HTTPMetric
		err = json.Unmarshal(bs, &one)
		arr = []HTTPMetric{one}
	}

	if err != nil {
		logger.Debugf("opentsdb msg format error: %s", err.Error())
		c.String(400, err.Error())
		return
	}

	var (
		succ int
		fail int
		msg  = "data pushed to queue"
		ts   = time.Now().Unix()
		ids  = make(map[string]interface{})
	)

	for i := 0; i < len(arr); i++ {
		if err := arr[i].Clean(ts); err != nil {
			logger.Debugf("opentsdb msg clean error: %s", err.Error())
			if fail == 0 {
				msg = fmt.Sprintf("%s , Error clean: %s", msg, err.Error())
			}
			fail++
			continue
		}

		pt, err := arr[i].ToProm()
		if err != nil {
			logger.Debugf("opentsdb msg to tsdb error: %s", err.Error())
			if fail == 0 {
				msg = fmt.Sprintf("%s , Error toprom: %s", msg, err.Error())
			}
			fail++
			continue
		}

		host, has := arr[i].Tags["ident"]
		if has {
			// register host
			ids[host] = ts

			// fill tags
			target, has := memsto.TargetCache.Get(host)
			if has {
				common.AppendLabels(pt, target)
			}
		}

		writer.Writers.PushSample(arr[i].Metric, pt)

		succ++
	}

	if succ > 0 {
		cn := config.ReaderClient.GetClusterName()
		if cn != "" {
			promstat.CounterSampleTotal.WithLabelValues(cn, "opentsdb").Add(float64(succ))
		}
		idents.Idents.MSet(ids)
	}

	if fail > 0 {
		logger.Debugf("opentsdb msg process error , msg is : %s", string(bs))
	}

	c.JSON(200, gin.H{
		"succ": succ,
		"fail": fail,
		"msg":  msg,
	})
}
