.PHONY: start build

NOW = $(shell date -u '+%Y%m%d%I%M%S')


APP 			= n9e
SERVER_BIN  	= $(APP)
ROOT:=$(shell pwd -P)
GIT_COMMIT:=$(shell git --work-tree ${ROOT}  rev-parse 'HEAD^{commit}')
_GIT_VERSION:=$(shell git --work-tree ${ROOT} describe --tags --abbrev=14 "${GIT_COMMIT}^{commit}" 2>/dev/null)
TAG=$(shell echo "${_GIT_VERSION}" |  awk -F"-" '{print $$1}')
RELEASE_VERSION:="$(TAG)-$(GIT_COMMIT)"

# RELEASE_ROOT 	= release
# RELEASE_SERVER 	= release/${APP}
# GIT_COUNT 		= $(shell git rev-list --all --count)
# GIT_HASH        = $(shell git rev-parse --short HEAD)
# RELEASE_TAG     = $(RELEASE_VERSION).$(GIT_COUNT).$(GIT_HASH)

all: build

build:
	go build -ldflags "-w -s -X github.com/didi/nightingale/v5/src/pkg/version.VERSION=$(RELEASE_VERSION)" -o $(SERVER_BIN) ./src

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "-w -s -X github.com/didi/nightingale/v5/src/pkg/version.VERSION=$(RELEASE_VERSION)" -o $(SERVER_BIN) ./src

# start:
# 	@go run -ldflags "-X main.VERSION=$(RELEASE_TAG)" ./cmd/${APP}/main.go web -c ./configs/config.toml -m ./configs/model.conf --menu ./configs/menu.yaml
run_webapi:
	nohup ./n9e webapi > webapi.log 2>&1 &

run_server:
	nohup ./n9e server > server.log 2>&1 &

# swagger:
# 	@swag init --parseDependency --generalInfo ./cmd/${APP}/main.go --output ./internal/app/swagger

# wire:
# 	@wire gen ./internal/app

# test:
# 	cd ./internal/app/test && go test -v

# clean:
# 	rm -rf data release $(SERVER_BIN) internal/app/test/data cmd/${APP}/data

pack: build
	rm -rf $(APP)-$(RELEASE_VERSION).tar.gz
	tar -zcvf $(APP)-$(RELEASE_VERSION).tar.gz docker etc $(SERVER_BIN) pub/font pub/index.html pub/assets pub/image
