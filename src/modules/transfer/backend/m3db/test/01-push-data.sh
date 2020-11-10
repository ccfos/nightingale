#!/bin/bash

# type MetricValue struct {
# 	Nid          string            `json:"nid"`
# 	Metric       string            `json:"metric"`
# 	Endpoint     string            `json:"endpoint"`
# 	Timestamp    int64             `json:"timestamp"`
# 	Step         int64             `json:"step"`
# 	ValueUntyped interface{}       `json:"value"`
# 	Value        float64           `json:"-"`
# 	CounterType  string            `json:"counterType"`
# 	Tags         string            `json:"tags"`
# 	TagsMap      map[string]string `json:"tagsMap"` //保留2种格式，方便后端组件使用
# 	Extra        string            `json:"extra"`
# }


curl -X POST  \
	http://localhost:8008/api/transfer/push \
-d '[{
  "metric": "test",
  "endpoint": "m3db-dev01-yubo.py",
  "timestamp": '$(date "+%s")',
  "step": 60,
  "value": 1.111,
  "counterType": "GAUGE",
  "tags": "",
  "tagsMap": {
    "city":"bj",
    "region":"c1",
    "test": "end"
   },
  "extra": ""
}]'

