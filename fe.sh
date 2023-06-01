#!/bin/bash

TAG=$(curl -sX GET https://api.github.com/repos/n9e/fe/releases/latest   | awk '/tag_name/{print $4;exit}' FS='[""]')
VERSION=$(echo $TAG)

echo aaa

export PATH=$PATH:$GOROOT/bin:$GOPATH/bin

echo bbb


if ! curl -o n9e-fe-${VERSION}.tar.gz -L https://github.com/n9e/fe/releases/download/${TAG}/n9e-fe-${VERSION}.tar.gz; then
    echo "failed to download n9e-fe-${VERSION}.tar.gz!"
    exit 2
fi

echo ccc


if ! tar zxvf n9e-fe-${VERSION}.tar.gz; then
    echo "failed to untar n9e-fe-${VERSION}.tar.gz!"
    exit 3
fi

echo ddd


cp ./docker/initsql/a-n9e.sql n9e.sql

echo eee


# Embed files into a Go executable
if ! statik -src=./pub -dest=./front; then
    echo "failed to embed files into a Go executable!"
    exit 4
fi

echo fff


# rm the fe file
rm n9e-fe-${VERSION}.tar.gz
rm -r ./pub
