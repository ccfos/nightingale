#!/bin/bash

# release version
version=5.0.0-rc3

export GO111MODULE=on
go build -ldflags "-X main.version=${version}" -o n9e-server main.go

