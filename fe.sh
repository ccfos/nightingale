#!/bin/bash

cp -f ./docker/initsql/a-n9e.sql n9e.sql

if [ ! -d "./pub" ]; then
    TAG=$(curl -sX GET https://api.github.com/repos/n9e/fe/releases/latest   | awk '/tag_name/{print $4;exit}' FS='[""]')

    if ! curl -o n9e-fe-${TAG}.tar.gz -L https://github.com/n9e/fe/releases/download/${TAG}/n9e-fe-${TAG}.tar.gz; then
        echo "failed to download n9e-fe-${TAG}.tar.gz!"
        exit 1
    fi

    if ! tar zxf n9e-fe-${TAG}.tar.gz; then
        echo "failed to untar n9e-fe-${TAG}.tar.gz!"
        exit 2
    fi
fi


# Embed files into a go binary
if ! $GOPATH/bin/statik -src=./pub -dest=./front; then
    echo "failed to embed files into a go binary!"
    exit 4
fi
