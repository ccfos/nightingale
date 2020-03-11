#!/bin/bash
# author: ulric.qin@gmail.com

duration=$(cat /proc/uptime | awk '{print $1}')
localip=$(/usr/sbin/ifconfig `/usr/sbin/route|grep '^default'|awk '{print $NF}'`|grep inet|awk '{print $2}'|head -n 1)
step=$(basename $0|awk -F'_' '{print $1}')
echo '[
    {
        "endpoint": "'${localip}'",
        "tags": "",
        "timestamp": '$(date +%s)',
        "metric": "sys.uptime.duration",
        "value": '${duration}',
        "step": '${step}'
    }
]'