#!/bin/bash

# release version
version=5.0.0-rc7-1

#export GO111MODULE=on
#export GOPROXY=https://goproxy.cn
go build -ldflags "-X github.com/didi/nightingale/v5/config.Version=${version}" -o n9e-server main.go

