#!/bin/bash

# type QueryData struct {
# 	Start      int64    `json:"start"`
# 	End        int64    `json:"end"`
# 	ConsolFunc string   `json:"consolFunc"`
# 	Endpoints  []string `json:"endpoints"`
# 	Nids       []string `json:"nids"`
# 	Counters   []string `json:"counters"`
# 	Step       int      `json:"step"`
# 	DsType     string   `json:"dstype"`
# }


curl -X POST  \
	http://localhost:8008/api/transfer/data \
-d '[{
  "start": '$(date -d "1 hour ago" "+%s")',
  "end": '$(date "+%s")',
  "consolFunc": "",
  "endpoints": ["m3db-dev01-yubo.py"],
  "counters": [],
  "step": 60,
  "dstype": "GAUGE"
}]' | jq .

