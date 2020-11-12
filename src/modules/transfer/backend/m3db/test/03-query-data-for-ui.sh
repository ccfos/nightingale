#!/bin/bash

# type QueryDataForUI struct {
# 	Start       int64    `json:"start"`
# 	End         int64    `json:"end"`
# 	Metric      string   `json:"metric"`
# 	Endpoints   []string `json:"endpoints"`
# 	Nids        []string `json:"nids"`
# 	Tags        []string `json:"tags"`
# 	Step        int      `json:"step"`
# 	DsType      string   `json:"dstype"`
# 	GroupKey    []string `json:"groupKey"` //聚合维度
# 	AggrFunc    string   `json:"aggrFunc"` //聚合计算
# 	ConsolFunc  string   `json:"consolFunc"`
# 	Comparisons []int64  `json:"comparisons"` //环比多少时间
# }




curl -X POST  \
	http://localhost:8008/api/transfer/data/ui \
-d '{
  "start": '$(date -d "1 hour ago" "+%s")',
  "end": '$(date "+%s")',
  "metric": "test",
  "endpoints": ["m3db-dev01-yubo.py"],
  "nids": [],
  "tags": [],
  "step": 60,
  "dstype": "",
  "groupKey": [],
  "aggrFunc": "",
  "consolFunc": "",
  "comparisons": []
}' | jq .


