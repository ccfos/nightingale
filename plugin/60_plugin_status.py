#!/usr/bin/env python
# -*- coding: utf-8 -*-
# author: ulric.qin@gmail.com

import time
import commands
import json
import sys
import os

items = []


def collect_myself_status():
    item = {}
    item["metric"] = "plugin.myself.status"
    item["value"] = 1
    item["tags"] = ""
    items.append(item)


def main():
    code, endpoint = commands.getstatusoutput(
        "timeout 1 /usr/sbin/ifconfig `/usr/sbin/route|grep '^default'|awk '{print $NF}'`|grep inet|awk '{print $2}'|head -n 1")
    if code != 0:
        sys.stderr.write('cannot get local ip')
        return

    timestamp = int("%d" % time.time())
    plugin_name = os.path.basename(sys.argv[0])
    step = int(plugin_name.split("_", 1)[0])

    collect_myself_status()

    for item in items:
        item["endpoint"] = endpoint
        item["timestamp"] = timestamp
        item["step"] = step

    print json.dumps(items)


if __name__ == "__main__":
    sys.stdout.flush()
    main()
    sys.stdout.flush()
