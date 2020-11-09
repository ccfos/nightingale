#!/bin/bash

# type EndpointMetricRecv struct {
# 	Endpoints []string `json:"endpoints"`
# 	Metrics   []string `json:"metrics"`
# }

curl -X POST  \
	http://localhost:8008/api/index/tagkv \
-d '{
  "endpoints": [],
  "metrics": ["test"]
}' | jq .


