#!/bin/sh

now=$(date +%s)

echo '[
    {
        "metric": "plugin_example_gauge",
        "tags": {
            "type": "testcase",
            "author": "ulric"
        },
        "value": '${now}',
        "type": "gauge"
    },
    {
        "metric": "plugin_example_rate",
        "tags": {
            "type": "testcase",
            "author": "ulric"
        },
        "value": '${now}',
        "type": "rate"
    },
    {
        "metric": "plugin_example_increase",
        "tags": {
            "type": "testcase",
            "author": "ulric"
        },
        "value": '${now}',
        "type": "increase"
    }
]'