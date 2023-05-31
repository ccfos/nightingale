#!/bin/bash

TAG=$(curl -sX GET https://api.github.com/repos/n9e/fe/releases/latest   | awk '/tag_name/{print $4;exit}' FS='[""]')
VERSION=$(echo $TAG)

curl -o n9e-fe-${VERSION}.tar.gz -L https://github.com/n9e/fe/releases/download/${TAG}/n9e-fe-${VERSION}.tar.gz  

tar zxvf n9e-fe-${VERSION}.tar.gz

cp ./docker/initsql/a-n9e.sql n9e.sql

# Embed files into a Go executable
statik -src=./pub -dest=./front

# rm the fe file
rm n9e-fe-${VERSION}.tar.gz
rm -r ./pub