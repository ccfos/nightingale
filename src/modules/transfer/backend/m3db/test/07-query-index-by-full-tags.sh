#!/bin/bash

# type IndexByFullTagsRecv struct {
# 	Endpoints []string  `json:"endpoints"`
# 	Metric    string    `json:"metric"`
# 	Tagkv     []TagPair `json:"tagkv"`
# }

curl -X POST  \
	http://localhost:8008/api/index/counter/fullmatch \
-d '[{
  "endpoints": ["m3db-dev01-yubo.py"],
  "metric": "test2",
  "tagkv": []
}]' | jq .



