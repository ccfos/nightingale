export GO15VENDOREXPERIMENT=1

PATH := $(GOPATH)/bin:$(PATH)
EXAMPLES=./examples/bench/server ./examples/bench/client ./examples/ping ./examples/thrift ./examples/hyperbahn/echo-server
ALL_PKGS := $(shell glide nv)
PROD_PKGS := . ./http ./hyperbahn ./json ./peers ./pprof ./raw ./relay ./stats ./thrift $(EXAMPLES)
TEST_ARG ?= -race -v -timeout 5m
BUILD := ./build
THRIFT_GEN_RELEASE := ./thrift-gen-release
THRIFT_GEN_RELEASE_LINUX := $(THRIFT_GEN_RELEASE)/linux-x86_64
THRIFT_GEN_RELEASE_DARWIN := $(THRIFT_GEN_RELEASE)/darwin-x86_64

PLATFORM := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH := $(shell uname -m)

OLD_GOPATH := $(GOPATH)

BIN := $(shell pwd)/.bin

# Cross language test args
TEST_HOST=127.0.0.1
TEST_PORT=0

-include crossdock/rules.mk

all: test examples

$(BIN)/thrift:
	mkdir -p $(BIN)
	scripts/install-thrift.sh $(BIN)

packages_test:
	go list -json ./... | jq -r '. | select ((.TestGoFiles | length) > 0)  | .ImportPath'

setup:
	mkdir -p $(BUILD)
	mkdir -p $(BUILD)/examples
	mkdir -p $(THRIFT_GEN_RELEASE_LINUX)
	mkdir -p $(THRIFT_GEN_RELEASE_DARWIN)

# We want to remove `vendor` dir because thrift-gen tests don't work with it.
# However, glide install even with --cache-gopath option leaves GOPATH at HEAD,
# not at the desired versions from glide.lock, which are only applied to `vendor`
# dir. So we move `vendor` to a temp dir and prepend it to GOPATH.
# Note that glide itself is still executed against the original GOPATH.
install:
	GOPATH=$(OLD_GOPATH) glide --debug install --cache --cache-gopath

install_lint:
	@echo "Installing golint, since we expect to lint"
	GOPATH=$(OLD_GOPATH) go get -u -f golang.org/x/lint/golint

install_glide:
	# all we want is: GOPATH=$(OLD_GOPATH) go get -u github.com/Masterminds/glide
	# but have to pin to 0.12.3 due to https://github.com/Masterminds/glide/issues/745
	GOPATH=$(OLD_GOPATH) go get -u github.com/Masterminds/glide && cd $(OLD_GOPATH)/src/github.com/Masterminds/glide && git checkout v0.12.3 && go install

install_ci: $(BIN)/thrift install_glide install
ifdef CROSSDOCK
	$(MAKE) install_docker_ci
endif

install_test:
	go test -i $(TEST_ARG) $(ALL_PKGS)

help:
	@egrep "^# target:" [Mm]akefile | sort -

clean:
	echo Cleaning build artifacts...
	go clean
	rm -rf $(BUILD) $(THRIFT_GEN_RELEASE)
	echo

fmt format:
	echo Formatting Packages...
	go fmt $(ALL_PKGS)
	echo

test_ci:
ifdef CROSSDOCK
	$(MAKE) crossdock_ci
else
	$(MAKE) test
endif

test: clean setup install_test check_no_test_deps $(BIN)/thrift
	@echo Testing packages:
	PATH=$(BIN):$$PATH go test -parallel=4 $(TEST_ARG) $(ALL_PKGS)
	@echo Running frame pool tests
	PATH=$(BIN):$$PATH go test -run TestFramesReleased -stressTest $(TEST_ARG)

check_no_test_deps:
	! go list -json $(PROD_PKGS) | jq -r '.Deps | select ((. | length) > 0) | .[]' | grep -e test -e mock | grep -v '^internal/testlog'

benchmark: clean setup $(BIN)/thrift
	echo Running benchmarks:
	PATH=$(BIN)::$$PATH go test $(ALL_PKGS) -bench=. -cpu=1 -benchmem -run NONE

cover_profile: clean setup $(BIN)/thrift
	@echo Testing packages:
	mkdir -p $(BUILD)
	PATH=$(BIN)::$$PATH go test ./ $(TEST_ARG) -coverprofile=$(BUILD)/coverage.out

cover: cover_profile
	go tool cover -html=$(BUILD)/coverage.out

cover_ci:
	@echo "Uploading coverage"
	$(MAKE) cover_profile
	curl -s https://codecov.io/bash > $(BUILD)/codecov.bash
	bash $(BUILD)/codecov.bash -f $(BUILD)/coverage.out


FILTER := grep -v -e '_string.go' -e '/gen-go/' -e '/mocks/' -e 'vendor/'
lint:
	@echo "Running golint"
	-golint $(ALL_PKGS) | $(FILTER) | tee lint.log
	@echo "Running go vet"
	-go vet $(ALL_PKGS) 2>&1 | fgrep -v -e "possible formatting directiv" -e "exit status" | tee -a lint.log
	@echo "Verifying files are gofmt'd"
	-gofmt -l . | $(FILTER) | tee -a lint.log
	@echo "Checking for unresolved FIXMEs"
	-git grep -i -n fixme | $(FILTER) | grep -v -e Makefile | tee -a lint.log
	@[ ! -s lint.log ]

thrift_example: thrift_gen
	go build -o $(BUILD)/examples/thrift       ./examples/thrift/main.go

test_server:
	./build/examples/test_server --host ${TEST_HOST} --port ${TEST_PORT}

examples: clean setup thrift_example
	echo Building examples...
	mkdir -p $(BUILD)/examples/ping $(BUILD)/examples/bench
	go build -o $(BUILD)/examples/ping/pong    ./examples/ping/main.go
	go build -o $(BUILD)/examples/hyperbahn/echo-server    ./examples/hyperbahn/echo-server/main.go
	go build -o $(BUILD)/examples/bench/server ./examples/bench/server
	go build -o $(BUILD)/examples/bench/client ./examples/bench/client
	go build -o $(BUILD)/examples/bench/runner ./examples/bench/runner.go
	go build -o $(BUILD)/examples/test_server ./examples/test_server

thrift_gen: $(BIN)/thrift
	go build -o $(BUILD)/thrift-gen ./thrift/thrift-gen
	PATH=$(BIN):$$PATH $(BUILD)/thrift-gen --generateThrift --inputFile thrift/test.thrift --outputDir thrift/gen-go/
	PATH=$(BIN):$$PATH $(BUILD)/thrift-gen --generateThrift --inputFile examples/keyvalue/keyvalue.thrift --outputDir examples/keyvalue/gen-go
	PATH=$(BIN):$$PATH $(BUILD)/thrift-gen --generateThrift --inputFile examples/thrift/example.thrift --outputDir examples/thrift/gen-go
	PATH=$(BIN):$$PATH $(BUILD)/thrift-gen --generateThrift --inputFile hyperbahn/hyperbahn.thrift --outputDir hyperbahn/gen-go

release_thrift_gen: clean setup
	GOOS=linux GOARCH=amd64 go build -o $(THRIFT_GEN_RELEASE_LINUX)/thrift-gen ./thrift/thrift-gen
	GOOS=darwin GOARCH=amd64 go build -o $(THRIFT_GEN_RELEASE_DARWIN)/thrift-gen ./thrift/thrift-gen
	tar -czf thrift-gen-release.tar.gz $(THRIFT_GEN_RELEASE)
	mv thrift-gen-release.tar.gz $(THRIFT_GEN_RELEASE)/

.PHONY: all help clean fmt format install install_ci install_lint install_glide release_thrift_gen packages_test check_no_test_deps test test_ci lint
.SILENT: all help clean fmt format test lint
