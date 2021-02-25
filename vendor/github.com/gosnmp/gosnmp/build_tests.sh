#!/usr/bin/env bash

snmpsimd.py --logging-method=null --agent-udpv4-endpoint=127.0.0.1:1024 &
go test -v -tags helper
go test -v -tags marshal
go test -v -tags misc
go test -v -tags api
go test -v -tags end2end
go test -v -tags trap
go test -v -tags all -race
