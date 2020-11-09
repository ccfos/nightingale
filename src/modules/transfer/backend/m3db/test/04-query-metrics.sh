#!/bin/bash

# type EndpointsRecv struct {
# 	Endpoints []string `json:"endpoints"`
# }

curl -X POST  \
	http://localhost:8008/api/index/metrics \
-d '{
  "endpoints": []
}'


