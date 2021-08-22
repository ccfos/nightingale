package prom

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	promapi "github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v2"
)

type PromWriteClientList []*PromWriteClient

type PromWriteClient struct {
	conf   PromClientConfig
	client promapi.Client
}

func (prom *PromDataSource) InitWriteClient() error {
	conf := prom.Section.RemoteWrite
	clients := make([]*PromWriteClient, 0, len(conf))
	for _, wc := range conf {

		c, err := newWriteClient(wc)
		if err != nil {
			logger.Errorf("init new write client got error: %s, config: %+v", err.Error(), wc)
			return err
		}

		clients = append(clients, c)
	}

	prom.WriteClients = clients
	return nil
}

func newWriteClient(conf PromClientConfig) (*PromWriteClient, error) {
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

	return &PromWriteClient{
		conf:   conf,
		client: c,
	}, nil
}

func (list PromWriteClientList) RemoteWriteToList(data []interface{}) int {
	errCnt := 0

	var items []*prompb.TimeSeries
	for _, dataItem := range data {
		item, ok := dataItem.(*prompb.TimeSeries)
		if !ok {
			errCnt++
			logger.Warningf("push data to prom but can't convert. data: %+v", dataItem)
			continue
		}
		items = append(items, item)
	}

	logger.Debugf("push data to prom: %+v", items)
	for _, client := range list {
		go client.RemoteWrite(items)
	}

	return errCnt
}

func (c *PromWriteClient) RemoteWrite(items []*prompb.TimeSeries) error {
	req := &prompb.WriteRequest{
		Timeseries: items,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		logger.Warningf("marshal prom data to proto got error: %s, data: %+v", err.Error(), req)
		return err
	}

	bs := snappy.Encode(nil, data)

	return c.Push(bs)
}

func (c *PromWriteClient) Push(req []byte) error {
	httpReq, err := http.NewRequest("POST", c.conf.URL.String(), bytes.NewReader(req))
	if err != nil {
		logger.Warningf("create remote write request got error: %s", err.Error())
		return err
	}

	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", "n9e")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	resp, body, err := c.client.Do(context.Background(), httpReq)
	if err != nil {
		logger.Warningf("push data with remote write request got error: %s, response body: %s", err.Error(), body)
		return err
	}

	if resp.StatusCode >= 400 {
		logger.Warningf("push data with remote write request bad response: %s %s", resp.Status, body)
		return fmt.Errorf("remote write got HTTP status: %d error: %w", resp.StatusCode, ErrWrongResponse)
	}

	return nil
}

func toHash(data interface{}) (string, error) {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(bytes)
	return hex.EncodeToString(hash[:]), nil
}
