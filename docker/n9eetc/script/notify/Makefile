
.phony: all
all: plugin

.phony: plugin
plugin:
	export GOPROXY=http://goproxy.cn,direct
	go build -buildmode=plugin -o notify.so notify.go

