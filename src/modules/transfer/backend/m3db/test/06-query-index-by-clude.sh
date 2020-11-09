#!/bin/bash


# type CludeRecv struct {
# 	Endpoints []string   `json:"endpoints"`
# 	Metric    string     `json:"metric"`
# 	Include   []*TagPair `json:"include"`
# 	Exclude   []*TagPair `json:"exclude"`
# }


curl -X POST  \
	http://localhost:8008/api/index/counter/clude \
-d '[{
  "endpoints": [],
  "metric": "test",
  "include": [],
  "exclude": [{"tagk":"city", "tagv": ["bjo"]}]
}]' | jq .
