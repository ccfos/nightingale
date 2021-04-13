# go-ping
[![PkgGoDev](https://pkg.go.dev/badge/github.com/go-ping/ping)](https://pkg.go.dev/github.com/go-ping/ping)
[![Circle CI](https://circleci.com/gh/go-ping/ping.svg?style=svg)](https://circleci.com/gh/go-ping/ping)

A simple but powerful ICMP echo (ping) library for Go, inspired by
[go-fastping](https://github.com/tatsushid/go-fastping).

Here is a very simple example that sends and receives three packets:

```go
pinger, err := ping.NewPinger("www.google.com")
if err != nil {
	panic(err)
}
pinger.Count = 3
err = pinger.Run() // Blocks until finished.
if err != nil {
	panic(err)
}
stats := pinger.Statistics() // get send/receive/rtt stats
```

Here is an example that emulates the traditional UNIX ping command:

```go
pinger, err := ping.NewPinger("www.google.com")
if err != nil {
	panic(err)
}

// Listen for Ctrl-C.
c := make(chan os.Signal, 1)
signal.Notify(c, os.Interrupt)
go func() {
	for _ = range c {
		pinger.Stop()
	}
}()

pinger.OnRecv = func(pkt *ping.Packet) {
	fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v\n",
		pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt)
}

pinger.OnFinish = func(stats *ping.Statistics) {
	fmt.Printf("\n--- %s ping statistics ---\n", stats.Addr)
	fmt.Printf("%d packets transmitted, %d packets received, %v%% packet loss\n",
		stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss)
	fmt.Printf("round-trip min/avg/max/stddev = %v/%v/%v/%v\n",
		stats.MinRtt, stats.AvgRtt, stats.MaxRtt, stats.StdDevRtt)
}

fmt.Printf("PING %s (%s):\n", pinger.Addr(), pinger.IPAddr())
err = pinger.Run()
if err != nil {
	panic(err)
}
```

It sends ICMP Echo Request packet(s) and waits for an Echo Reply in
response. If it receives a response, it calls the `OnRecv` callback.
When it's finished, it calls the `OnFinish` callback.

For a full ping example, see
[cmd/ping/ping.go](https://github.com/go-ping/ping/blob/master/cmd/ping/ping.go).

## Installation

```
go get -u github.com/go-ping/ping
```

To install the native Go ping executable:

```bash
go get -u github.com/go-ping/ping/...
$GOPATH/bin/ping
```

## Supported Operating Systems

### Linux
This library attempts to send an "unprivileged" ping via UDP. On Linux,
this must be enabled with the following sysctl command:

```
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"
```

If you do not wish to do this, you can call `pinger.SetPrivileged(true)`
in your code and then use setcap on your binary to allow it to bind to
raw sockets (or just run it as root):

```
setcap cap_net_raw=+ep /path/to/your/compiled/binary
```

See [this blog](https://sturmflut.github.io/linux/ubuntu/2015/01/17/unprivileged-icmp-sockets-on-linux/)
and the Go [x/net/icmp](https://godoc.org/golang.org/x/net/icmp) package
for more details.

### Windows

You must use `pinger.SetPrivileged(true)`, otherwise you will receive
the following error:

```
socket: The requested protocol has not been configured into the system, or no implementation for it exists.
```

Despite the method name, this should work without the need to elevate
privileges and has been tested on Windows 10. Please note that accessing
packet TTL values is not supported due to limitations in the Go
x/net/ipv4 and x/net/ipv6 packages.

### Plan 9 from Bell Labs

There is no support for Plan 9. This is because the entire `x/net/ipv4` 
and `x/net/ipv6` packages are not implemented by the Go programming 
language.

## Maintainers and Getting Help:

This repo was originally in the personal account of
[sparrc](https://github.com/sparrc), but is now maintained by the
[go-ping organization](https://github.com/go-ping).

For support and help, you usually find us in the #go-ping channel of
Gophers Slack. See https://invite.slack.golangbridge.org/ for an invite
to the Gophers Slack org.
