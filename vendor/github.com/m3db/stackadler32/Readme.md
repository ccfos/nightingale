# stackadler32

## Note: This is a fork of [github.com/sent-hil/adler32](http://github.com/sent-hil/adler32) that provides digests that are allocated on the stack and can be incrementally written to. This is useful for places where you perform concurrent checksumming and there's no good place to cache a digest without needing to acquire it expensively (under lock, etc).

Port of adler32 checksum function as described here: https://www.ietf.org/rfc/rfc1950.txt to Go.

## Example:

```go
 adler32.Checksum([]byte("Hello World"))
```

## Tests

```bash
$ go test
PASS
ok      github.com/sent-hil/adler32     2.429s

$ go test -bench=.
# This library is slightly faster than the one in standard library.
$ go test -bench=.
BenchmarkThis-4            10000            230169 ns/op
BenchmarkStdLib-4          10000            190834 ns/op
PASS
ok      github.com/sent-hil/adler32     6.554s
```
