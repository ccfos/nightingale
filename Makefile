.PHONY: prebuild build

ROOT:=$(shell pwd -P)
GIT_COMMIT:=$(shell git --work-tree ${ROOT}  rev-parse 'HEAD^{commit}')
_GIT_VERSION:=$(shell git --work-tree ${ROOT} describe --tags --abbrev=14 "${GIT_COMMIT}^{commit}" 2>/dev/null)
TAG=$(shell echo "${_GIT_VERSION}" |  awk -F"-" '{print $$1}')
RELEASE_VERSION:="$(TAG)-$(GIT_COMMIT)"

all: prebuild build

prebuild:
	echo "begin download and embed the front-end file..."
	sh fe.sh
	echo "front-end file download and embedding completed."

build:
	go build -ldflags "-w -s -X github.com/ccfos/nightingale/v6/pkg/version.Version=$(RELEASE_VERSION)" -o n9e ./cmd/center/main.go

build-edge:
	go build -ldflags "-w -s -X github.com/ccfos/nightingale/v6/pkg/version.Version=$(RELEASE_VERSION)" -o n9e-edge ./cmd/edge/

build-alert:
	go build -ldflags "-w -s -X github.com/ccfos/nightingale/v6/pkg/version.Version=$(RELEASE_VERSION)" -o n9e-alert ./cmd/alert/main.go

build-pushgw:
	go build -ldflags "-w -s -X github.com/ccfos/nightingale/v6/pkg/version.Version=$(RELEASE_VERSION)" -o n9e-pushgw ./cmd/pushgw/main.go

build-cli: 
	go build -ldflags "-w -s -X github.com/ccfos/nightingale/v6/pkg/version.Version=$(RELEASE_VERSION)" -o n9e-cli ./cmd/cli/main.go

run:
	nohup ./n9e > n9e.log 2>&1 &

run-alert:
	nohup ./n9e-alert > n9e-alert.log 2>&1 &

run-pushgw:
	nohup ./n9e-pushgw > n9e-pushgw.log 2>&1 &

release:
	goreleaser --skip-validate --skip-publish --snapshot