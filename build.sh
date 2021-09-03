#!/bin/bash

# release version
version=5.0.0-rc7

#export GO111MODULE=on
#export GOPROXY=https://goproxy.cn
go build -ldflags "-X main.version=${version}" -o n9e-server main.go

