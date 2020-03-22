FROM golang:alpine AS builder
RUN apk add --no-cache git
WORKDIR /home/app

# comment this if using vendor
# ENV GOPROXY=https://mod.gokit.info
# COPY go.mod go.sum ./
# RUN go mod download

COPY . .
ENV GOPROXY=https://mod.gokit.info
RUN go build -o ./bin/monapi src/modules/monapi/monapi.go

FROM alpine:3.10
LABEL maintainer="llitfkitfk@gmail.com"
RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /home/app/etc /app/etc
COPY --from=builder /home/app/bin /usr/local/bin

# ENTRYPOINT []
# CMD []