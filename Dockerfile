FROM golang AS builder
# RUN apk add --no-cache git gcc
WORKDIR /app

# comment this if using vendor
# ENV GOPROXY=https://mod.gokit.info
# COPY go.mod go.sum ./
# RUN go mod download

COPY . .
ENV GOPROXY=https://mod.gokit.info
RUN ./control build docker

FROM buildpack-deps:buster-curl
LABEL maintainer="llitfkitfk@gmail.com"

WORKDIR /app

COPY --from=builder /app/docker/scripts /app/scripts
COPY --from=builder /app/etc /app/etc
# Change default address (hard code) 
RUN ./scripts/sed.sh

COPY --from=builder /app/bin /usr/local/bin


# ENTRYPOINT []
# CMD []