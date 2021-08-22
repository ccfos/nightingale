package prom

import (
	"context"
	"math/rand"
	"time"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	promapi "github.com/prometheus/client_golang/api"

	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
)

type PromReadClientList []*PromReadClient

type PromReadClient struct {
	conf   PromClientConfig
	client API
}

func (prom *PromDataSource) InitReadClient() error {
	conf := prom.Section.RemoteRead
	clients := make([]*PromReadClient, 0, len(conf))
	for _, wc := range conf {

		c, err := newReadClient(wc)
		if err != nil {
			logger.Errorf("init new write client got error: %s, config: %+v", err.Error(), wc)
			return err
		}

		clients = append(clients, c)
	}

	prom.ReadClients = clients
	return nil
}

func newReadClient(conf PromClientConfig) (*PromReadClient, error) {
	name := conf.Name
	if name == "" {
		hash, err := toHash(conf)
		if err != nil {
			return nil, err
		}

		name = hash[:6]
	}

	c, err := promapi.NewClient(promapi.Config{
		Address: conf.URL.String(),
	})
	if err != nil {
		return nil, err
	}

	return &PromReadClient{
		conf:   conf,
		client: NewAPI(c),
	}, nil
}

func (list PromReadClientList) LabelValues(lname string, matchs []metricSelector) ([]string, error) {
	var matchStrs []string
	for _, m := range matchs {
		matchStrs = append(matchStrs, m.String())
	}

	var vals model.LabelValues
	var err error
	for _, idx := range rand.Perm(len(list)) {
		vals, err = list[idx].LabelValues(lname, matchStrs)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	var ret []string
	for _, val := range vals {
		ret = append(ret, string(val))
	}
	return ret, nil
}

func (list PromReadClientList) GetSeries(matchs []metricSelector, start, end int64) ([]MetricMeta, error) {
	var matchStrs []string
	for _, m := range matchs {
		matchStrs = append(matchStrs, m.String())
	}
	var startTime, endTime time.Time
	if end <= 0 {
		endTime = time.Now()
	} else {
		endTime = time.Unix(end, 0)
	}
	if start <= 0 || start >= end {
		startTime = endTime.AddDate(0, 0, -3)
	} else {
		startTime = time.Unix(start, 0)
	}

	var labels []model.LabelSet
	var err error
	for _, idx := range rand.Perm(len(list)) {
		labels, err = list[idx].GetSeries(matchStrs, startTime, endTime)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	var metrics []MetricMeta
	for _, lset := range labels {
		metrics = append(metrics, getMetricInfos(model.Metric(lset)))
	}
	return metrics, nil
}

func (list PromReadClientList) QueryRange(metric metricSelector, start, end int64, step int) ([]*dataobj.TsdbQueryResponse, error) {
	metricStr := metric.String()
	startTime := time.Unix(start, 0)
	endTime := time.Unix(end, 0)
	stepDur := time.Second * time.Duration(step)

	r := Range{
		Start: startTime,
		End:   endTime,
		Step:  stepDur,
	}
	r.Validate()

	var val model.Value
	var err error
	for _, idx := range rand.Perm(len(list)) {
		val, err = list[idx].QueryRange(metricStr, r)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	resp := convertValuesToRRDResp(val)

	for i := range resp {
		resp[i].Start = start
		resp[i].End = end
		resp[i].Step = step
	}

	return resp, err
}

func (c *PromReadClient) LabelValues(lname string, matchs []string) (model.LabelValues, error) {
	vals, warn, err := c.client.LabelValues(context.Background(), lname, matchs)
	if err != nil {
		logger.Errorf("query label values got error: %s, client: %s, label name: %s", err, c.conf.Name, lname)
		return nil, err
	}
	if len(warn) > 0 {
		logger.Warningf("get metrics name got warnings: %+v, client: %s, label name: %s", warn, c.conf.Name, lname)
	}

	return vals, nil
}

func (c *PromReadClient) GetSeries(matchs []string, start, end time.Time) ([]model.LabelSet, error) {
	labels, warn, err := c.client.Series(context.Background(), matchs, start, end)
	if err != nil {
		logger.Errorf("query series got error: %s, client: %s, matchs: %+v", err, c.conf.Name, matchs)
		return nil, err
	}
	if len(warn) > 0 {
		logger.Warningf("query series got warnings: %+v, client: %s, matchs: %+v", warn, c.conf.Name, matchs)
	}

	return labels, nil
}

func (c *PromReadClient) QueryRange(metric string, r Range) (model.Value, error) {
	values, warn, err := c.client.QueryRange(context.Background(), metric, r)
	if err != nil {
		logger.Warningf("query metrics range data got error: %s, client: %s, metric: %+s", err, c.conf.Name, metric)
		return nil, err
	}
	if len(warn) > 0 {
		logger.Warningf("query metrics range data got warnings: %+v, client: %s, metric: %s", warn, c.conf.Name, metric)
	}
	return values, nil
}
