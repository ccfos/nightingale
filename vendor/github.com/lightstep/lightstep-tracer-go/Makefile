# tools
GO = GO111MODULE=on GOPROXY=https://proxy.golang.org go

.PHONY: default build test install

default: build

build: version.go
	${GO} build ./...

test: build
	${GO} test -v -race ./...

install: test
	${GO} install ./...

clean:
	${GO} clean ./...

# When releasing significant changes, make sure to update the semantic
# version number in `./VERSION`, merge changes, then run `make release_tag`.
version.go: VERSION
	./tag_version.sh

release_tag:
	git tag -a v`cat ./VERSION`
	git push origin v`cat ./VERSION`
