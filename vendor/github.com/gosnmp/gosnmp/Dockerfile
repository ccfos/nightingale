FROM golang:1.14.4-alpine3.12

# Install deps
RUN apk add --no-cache  \
        bash            \
        curl            \
        gcc             \
        libc-dev        \
        make            \
        python3         \
        py3-pip

# add new user
RUN addgroup -g 1001                \
             -S gosnmp;             \
    adduser -u 1001 -D -S           \
            -s /bin/bash            \
            -h /home/gosnmp         \
            -G gosnmp gosnmp

RUN pip install snmpsim

# Copy local branch into container
USER gosnmp
WORKDIR /go/src/github.com/gosnmp/gosnmp
COPY --chown=gosnmp . .

RUN go get github.com/stretchr/testify/assert && \
    make tools && \
    make lint

ENV GOSNMP_TARGET=127.0.0.1
ENV GOSNMP_PORT=1024
ENV GOSNMP_TARGET_IPV4=127.0.0.1
ENV GOSNMP_PORT_IPV4=1024
ENV GOSNMP_TARGET_IPV6='::1'
ENV GOSNMP_PORT_IPV6=1024

ENTRYPOINT ["/go/src/github.com/gosnmp/gosnmp/build_tests.sh"]
