# go-ping
[![GoDoc](https://godoc.org/github.com/sparrc/go-ping?status.svg)](https://godoc.org/github.com/sparrc/go-ping)
[![Circle CI](https://circleci.com/gh/sparrc/go-ping.svg?style=svg)](https://circleci.com/gh/sparrc/go-ping)

ICMP Ping library for Go, inspired by
[go-fastping](https://github.com/tatsushid/go-fastping)

Here is a very simple example that sends & receives 3 packets:

```go
pinger, err := ping.NewPinger("www.google.com")
if err != nil {
        panic(err)
}
pinger.Count = 3
pinger.Run() // blocks until finished
stats := pinger.Statistics() // get send/receive/rtt stats
```

Here is an example that emulates the unix ping command:

```go
pinger, err := ping.NewPinger("www.google.com")
if err != nil {
        panic(err)
}

// listen for ctrl-C signal
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
pinger.Run()
```

It sends ICMP packet(s) and waits for a response. If it receives a response,
it calls the "receive" callback. When it's finished, it calls the "finish"
callback.

For a full ping example, see
[cmd/ping/ping.go](https://github.com/sparrc/go-ping/blob/master/cmd/ping/ping.go)

## Installation:

```
go get github.com/sparrc/go-ping
```

To install the native Go ping executable:

```bash
go get github.com/sparrc/go-ping/...
$GOPATH/bin/ping
```

## Note on Linux Support:

This library attempts to send an
"unprivileged" ping via UDP. On linux, this must be enabled by setting

```
sudo sysctl -w net.ipv4.ping_group_range="0   2147483647"
```

If you do not wish to do this, you can set `pinger.SetPrivileged(true)` and
use setcap to allow your binary using go-ping to bind to raw sockets
(or just run as super-user):

```
setcap cap_net_raw=+ep /bin/go-ping
```

See [this blog](https://sturmflut.github.io/linux/ubuntu/2015/01/17/unprivileged-icmp-sockets-on-linux/)
and [the Go icmp library](https://godoc.org/golang.org/x/net/icmp) for more details.

## Note on Windows Support:

You must use `pinger.SetPrivileged(true)`, otherwise you will receive an error:

```
Error listening for ICMP packets: socket: The requested protocol has not been configured into the system, or no implementation for it exists.
```

This should without admin privileges. Tested on Windows 10.

