FROM golang AS builder
# RUN apk add --no-cache git gcc
WORKDIR /app

# comment this if using vendor
# ENV GOPROXY=https://mod.gokit.info
# COPY go.mod go.sum ./
# RUN go mod download

COPY . .
ENV GOPROXY=https://mod.gokit.info
RUN echo "build monapi" \
  && go build -v -o ./bin/monapi src/modules/monapi/monapi.go \
  && echo "build transfer" \
  && go build -v -o ./bin/transfer src/modules/transfer/transfer.go \
  && echo "build tsdb" \
  && go build -v -o ./bin/tsdb src/modules/tsdb/tsdb.go \
  && echo "build index" \
  && go build -v -o ./bin/index src/modules/index/index.go \
  && echo "build judge" \
  && go build -v -o ./bin/judge src/modules/judge/judge.go \
  && echo "build collector" \
  && go build -v -o ./bin/collector src/modules/collector/collector.go

FROM alpine:3.10
LABEL maintainer="llitfkitfk@gmail.com"
RUN apk add --no-cache tzdata ca-certificates bash

WORKDIR /app

COPY --from=builder /app/etc /app/etc
COPY --from=builder /app/bin /usr/local/bin


# ENTRYPOINT []
# CMD []