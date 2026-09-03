# net_response

A network probing plugin, typically used to monitor whether a local port is listening or whether a remote port is reachable. Since the time-series databases in the Prometheus ecosystem can only store float64 values, the probe result is also a float64 value, but its meaning varies as follows:

```
- 0: Success
- 1: Timeout
- 2: ConnectionFailed
- 3: ReadFailed
- 4: StringMismatch
```

If everything is fine, the value is 0; if something goes wrong, the value is between 1 and 4, with the meanings above. The metric name for this value is `net_response_result_code`.

## Configuration

`conf/input.net_response/net_response.toml` in categraf. The most essential setting is the targets section, which specifies the probe targets, for example:

```toml
[[instances]]
targets = [
    "10.2.3.4:22",
    "localhost:6379",
    ":9090"
]
```

- `10.2.3.4:22` probes whether port 22 on host 10.2.3.4 is reachable
- `localhost:6379` probes whether port 6379 on the local machine is reachable
- `:9090` probes whether port 9090 on the local machine is reachable

The monitoring data or alert events only contain an IP and a port, so the person receiving the alert may not know which business module is affected. You can attach more meaningful information as labels, for example:

```toml
labels = { region="cloud", product="n9e" }
```

This identifies the region as cloud and the product as n9e. These two labels will be attached to the time-series data and will naturally show up in alerts as well.

A complete configuration example is shown below:

```toml
[mappings]
# "127.0.0.1:22"= {region="local",ssh="test"}
# "127.0.0.1:22"= {region="local",ssh="redis"}

[[instances]]
targets = [
#     "127.0.0.1:22",
#     "localhost:6379",
#     ":9090"
]

# # append some labels for series
# labels = { region="cloud", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

## Protocol, must be "tcp" or "udp"
## NOTE: because the "udp" protocol does not respond to requests, it requires
## a send/expect string pair (see below).
# protocol = "tcp"

## Set timeout
# timeout = "1s"

## Set read timeout (only used if expecting a response)
# read_timeout = "1s"

## The following options are required for UDP checks. For TCP, they are
## optional. The plugin will send the given string to the server and then
## expect to receive the given 'expect' string back.
## string sent to the server
# send = "ssh"
## expected string in answer
# expect = "ssh"
```
