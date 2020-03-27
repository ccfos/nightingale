FROM golang AS builder
# RUN apk add --no-cache git gcc
WORKDIR /app

COPY . .
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