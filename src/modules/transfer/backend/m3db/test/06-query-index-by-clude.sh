#!/bin/bash

# data:[{Nid: Endpoint:10.86.76.13 Metric:disk.bytes.used.percent Tags:[mount=/tmp] Step:0 Dstype:}]

# type CludeRecv struct {
# 	Endpoints []string   `json:"endpoints"`
# 	Metric    string     `json:"metric"`
# 	Include   []*TagPair `json:"include"`
# 	Exclude   []*TagPair `json:"exclude"`
# }


curl -X POST  \
	http://localhost:8008/api/index/counter/clude \
-d '[{
  "endpoints": ["10.86.76.13"],
  "metric": "disk.bytes.used.percent",
  "exclude": [{"tagk":"mount", "tagv": ["/boot"]}],
  "include": [{"tagk":"mount", "tagv": ["/", "/home"]}]
}]' | jq .
