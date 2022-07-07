#!/bin/bash

TAG=$(curl -sX GET https://api.github.com/repos/n9e/fe-v5/releases/latest   | awk '/tag_name/{print $4;exit}' FS='[""]')
VERSION=$(echo $TAG | sed 's/v//g')

curl -o n9e-fe-${VERSION}.tar.gz -L https://github.com/n9e/fe-v5/releases/download/${TAG}/n9e-fe-${VERSION}.tar.gz  

tar zxvf n9e-fe-${VERSION}.tar.gz
