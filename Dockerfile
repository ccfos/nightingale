FROM golang AS builder
# RUN apk add --no-cache git gcc
WORKDIR /app

# comment this if using vendor
# ENV GOPROXY=https://mod.gokit.info
# COPY go.mod go.sum ./
# RUN go mod download

COPY . .
RUN ./control build

FROM buildpack-deps:buster-curl
LABEL maintainer="llitfkitfk@gmail.com"

WORKDIR /app

COPY --from=builder /app/docker/etc /app/etc
COPY --from=builder /app/bin /usr/local/bin


# ENTRYPOINT []
# CMD []