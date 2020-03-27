#!/bin/bash

set -xe

sed -i 's/127.0.0.1:6379/redis:6379/g' /app/etc/judge.yml
sed -i 's/127.0.0.1:6379/redis:6379/g' /app/etc/monapi.yml
sed -i 's/127.0.0.1:3306/mysql:3306/g' /app/etc/mysql.yml
sed -i 's/127.0.0.1:5821/tsdb:5821/g' /app/etc/transfer.yml