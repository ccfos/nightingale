# sockstat

Read sockstat info from /proc/net/sockstat and /proc/net/sockstat6

## example file
```shell
sockets: used 211
TCP: inuse 9 orphan 0 tw 19 alloc 47 mem 22
UDP: inuse 2 mem 0
UDPLITE: inuse 0
RAW: inuse 0
FRAG: inuse 0 memory 0
```

The content of "/proc/net/sockstat" in a Linux system provides information about the socket usage on the system. The fields and their meaning are as follows:
```shell

sockets: used: Total number of used sockets on the system.
TCP: inuse: Number of currently established TCP sockets.
orphan: Number of orphaned TCP sockets.
tw: Number of sockets in TIME_WAIT state.
alloc: Number of sockets allocated.
mem: Memory used by TCP sockets.
UDP: inuse: Number of currently established UDP sockets.
mem: Memory used by UDP sockets.
UDPLITE: inuse: Number of currently established UDP-Lite sockets.
RAW: inuse: Number of currently established raw sockets.
FRAG: inuse: Number of currently established fragment sockets.
memory: Memory used by fragment sockets.
These fields provide a snapshot of the socket usage on the system, including the number of sockets in use and memory usage, which can be useful for monitoring and troubleshooting network issues.
```
